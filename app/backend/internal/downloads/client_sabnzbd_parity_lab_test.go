//go:build lab

package downloads

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLabSabnzbdParity(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_SAB_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_SAB_KEY not set")
	}
	ips, err := net.LookupHost("sabnzbd")
	if err != nil || len(ips) == 0 {
		t.Fatalf("sabnzbd not on this network: %v", err)
	}
	base := "http://" + ips[0] + ":8080"
	me := ownAddress(t)
	nntp := startFakeNNTP(t, ":1119")
	nntp.delay = 400 * time.Millisecond
	data := make([]byte, 1_200_000)
	_, _ = rand.Read(data)
	good := "Lab.Parity.Movie.2024.1080p.WEB.H264-HOLYARRS"
	bad := "Lab.Parity.Broken.2024.1080p.WEB.H264-HOLYARRS"
	slow := "Lab.Parity.Slow.2024.1080p.WEB.H264-HOLYARRS"
	big := make([]byte, 6_000_000)
	_, _ = rand.Read(big)
	nzbs := map[string]string{
		"/good.nzb": nntp.makeNZB(good+".mkv", data, 200_000),
		"/bad.nzb":  strings.ReplaceAll(nntp.makeNZB(bad+".mkv", data[:300_000], 150_000), "part", "missing"),
		"/slow.nzb": nntp.makeNZB(slow+".mkv", big, 100_000),
	}
	ln, err := net.Listen("tcp", ":18799")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, ok := nzbs[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-nzb")
		_, _ = io.WriteString(w, n)
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	raw := func(mode string, q url.Values) {
		q.Set("mode", mode)
		q.Set("output", "json")
		q.Set("apikey", key)
		resp, err := http.Get(base + "/api?" + q.Encode())
		if err != nil {
			t.Fatalf("%s: %v", mode, strings.ReplaceAll(err.Error(), key, "KEY"))
		}
		_ = resp.Body.Close()
	}
	server := "holyarrs-lab-fake-news"
	raw("set_config", url.Values{"section": {"servers"}, "keyword": {server}, "host": {me}, "port": {"1119"}, "connections": {"2"},
		"ssl": {"0"}, "enable": {"1"}, "retention": {"0"}})
	defer raw("del_config", url.Values{"section": {"servers"}, "keyword": {server}})

	ctx := context.Background()
	s := NewSabnzbd(base, key, nil)
	cat := "movies"
	if old, err := s.List(ctx, ""); err == nil {
		for _, j := range old {
			if strings.HasPrefix(j.Name, "Lab.Parity.") {
				_ = s.Remove(ctx, j.ID, true)
			}
		}
	}
	ids := map[string]string{}
	for name, path := range map[string]string{good: "/good.nzb", bad: "/bad.nzb"} {
		id, err := s.Add(ctx, AddRequest{URL: "http://" + me + ":18799" + path, Category: cat, Name: name})
		if err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
		ids[name] = id
	}
	defer func() {
		for _, id := range ids {
			_ = s.Remove(context.Background(), id, true)
		}
	}()
	deadline := time.Now().Add(3 * time.Minute)
	for {
		g, gerr := s.Get(ctx, ids[good])
		b, berr := s.Get(ctx, ids[bad])
		if gerr == nil && berr == nil && g.State == StateFinished && b.State == StateFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not done in time: good %s (%s), bad %s (%s)", g.State, g.Detail, b.State, b.Detail)
		}
		time.Sleep(500 * time.Millisecond)
	}

	h, err := s.History(ctx, 0)
	if err != nil || !h.Changed || h.Last == 0 {
		t.Fatalf("history: %v changed=%v last=%d", err, h.Changed, h.Last)
	}
	seen := map[string]Job{}
	for _, j := range h.Jobs {
		seen[j.ID] = j
	}
	if seen[ids[good]].State != StateFinished || seen[ids[bad]].State != StateFailed || seen[ids[good]].DoneAt == 0 {
		t.Fatalf("history rows: good %+v bad %+v", seen[ids[good]], seen[ids[bad]])
	}
	again, err := s.History(ctx, h.Last)
	if err != nil || again.Changed || len(again.Jobs) != 0 {
		t.Fatalf("unchanged history: %v %+v", err, again)
	}
	t.Logf("finished list: %d rows, counter %d; unchanged read answered no rows", len(h.Jobs), h.Last)

	st, err := s.ServerStats(ctx)
	if err != nil {
		t.Fatalf("server stats: %v", err)
	}
	t.Logf("server stats: total %d, %d server(s)", st.Total, len(st.Servers))
	stages, err := s.JobStages(ctx, ids[bad])
	if err != nil || len(stages) == 0 {
		t.Fatalf("stages: %v %+v", err, stages)
	}
	for _, sg := range stages {
		for _, l := range sg.Lines {
			if strings.Contains(l, "?") && strings.Contains(l, "http") {
				t.Fatalf("an address kept its query: %q", l)
			}
		}
	}
	t.Logf("failed job stages: %+v", stages)

	nntp.delay = 3 * time.Second
	sid, err := s.Add(ctx, AddRequest{URL: "http://" + me + ":18799/slow.nzb", Category: cat, Name: slow})
	if err != nil {
		t.Fatalf("add slow: %v", err)
	}
	ids[slow] = sid
	waitState := func(id, want string) Job {
		t.Helper()
		end := time.Now().Add(30 * time.Second)
		for {
			q, err := s.Queue(ctx)
			if err != nil {
				t.Fatalf("queue: %v", err)
			}
			for _, j := range q.Jobs {
				if j.ID == id && (want == "" || j.State == want) {
					return j
				}
			}
			if time.Now().After(end) {
				t.Fatalf("%s never %q in the queue: %+v", id, want, q.Jobs)
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
	waitState(sid, "")
	if err := s.PauseItem(ctx, sid); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitState(sid, StatePaused)
	if err := s.ResumeItem(ctx, sid); err != nil {
		t.Fatalf("resume: %v", err)
	}
	j := waitState(sid, "")
	if j.State == StatePaused {
		t.Fatalf("still paused after resume")
	}
	if err := s.MoveTop(ctx, sid); err != nil {
		q, _ := s.Queue(ctx)
		b := rawBody(t, base, key, "switch", url.Values{"value": {sid}, "value2": {"0"}})
		t.Fatalf("move to top: %v (queue %+v, answer %s)", err, q.Jobs, b)
	}
	q, _ := s.Queue(ctx)
	t.Logf("queue: %d job(s), speed %d B/s, time left %d s, free %d B", len(q.Jobs), q.Head.Speed, q.Head.TimeLeft, q.Head.FreeSpace)
	if err := s.Remove(ctx, sid, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	delete(ids, slow)
	if _, err := s.Get(ctx, sid); err != ErrNotFound {
		t.Fatalf("slow job still there: %v", err)
	}

	nid, err := s.Retry(ctx, ids[bad])
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	t.Logf("retried %s as %s (new id: %v)", ids[bad], nid, nid != ids[bad])
	ids[bad] = nid
	end := time.Now().Add(time.Minute)
	for {
		if j, err := s.Get(ctx, nid); err == nil {
			t.Logf("after retry: %s (%s)", j.State, j.Detail)
			break
		}
		if time.Now().After(end) {
			t.Fatal("the retried job is in neither list")
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func rawBody(t *testing.T, base, key, mode string, q url.Values) string {
	q.Set("mode", mode)
	q.Set("output", "json")
	q.Set("apikey", key)
	resp, err := http.Get(base + "/api?" + q.Encode())
	if err != nil {
		return "error"
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return strings.ReplaceAll(string(b), key, "KEY")
}
