//go:build lab

package downloader

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Runs inside the lab network, from a container that SABnzbd can reach:
//
//	HOLYARRS_LAB_SAB_KEY=<lab key> go test -tags lab -run Sabnzbd ./internal/downloader/
//
// Our own news server (fakenntp_test.go) serves a made-up file; SABnzbd is
// pointed at it for the test only, and the server entry is removed again.
// One NZB finishes, one NZB with parts that do not exist fails. With
// HOLYARRS_RECORD=1 the replies are saved as testdata for the adapter tests.
func TestLabSabnzbd(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_SAB_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_SAB_KEY not set")
	}
	ips, err := net.LookupHost("sabnzbd")
	if err != nil || len(ips) == 0 {
		t.Fatalf("sabnzbd not on this network: %v", err)
	}
	// SABnzbd checks the Host name; an address passes.
	base := "http://" + ips[0] + ":8080"
	me := ownAddress(t)
	nntp := startFakeNNTP(t, ":1119")
	nntp.delay = 1500 * time.Millisecond
	data := make([]byte, 1_200_000)
	_, _ = rand.Read(data)
	good := "Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS"
	bad := "Lab.Usenet.Broken.2024.1080p.WEB.H264-HOLYARRS"
	nzbs := map[string]string{
		"/good.nzb": nntp.makeNZB(good+".mkv", data, 200_000),
		"/bad.nzb":  strings.ReplaceAll(nntp.makeNZB(bad+".mkv", data[:300_000], 150_000), "part", "missing"),
		"/next.nzb": nntp.makeNZB(labNext+".mkv", data[:400_000], 200_000),
		"/dead.nzb": strings.ReplaceAll(nntp.makeNZB(labDead+".mkv", data[:300_000], 150_000), "part", "missing"),
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

	ctx := context.Background()
	raw := func(mode string, q url.Values) []byte {
		q.Set("mode", mode)
		q.Set("output", "json")
		q.Set("apikey", key)
		resp, err := http.Get(base + "/api?" + q.Encode())
		if err != nil {
			t.Fatalf("%s: %v", mode, strings.ReplaceAll(err.Error(), key, "KEY"))
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return b
	}
	server := "holyarrs-lab-fake-news"
	raw("set_config", url.Values{"section": {"servers"}, "keyword": {server}, "host": {me}, "port": {"1119"}, "connections": {"2"},
		"ssl": {"0"}, "enable": {"1"}, "retention": {"0"}})
	defer raw("del_config", url.Values{"section": {"servers"}, "keyword": {server}})

	s := NewSabnzbd(base, key, nil)
	h, err := s.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	t.Logf("SABnzbd %s", h.Version)
	if _, err := NewSabnzbd(base, "not-the-key", nil).Health(ctx); err != ErrAuth {
		t.Fatalf("wrong key: %v", err)
	}
	record := os.Getenv("HOLYARRS_RECORD") == "1"
	save := func(name string, b []byte) {
		if !record {
			return
		}
		b = []byte(strings.ReplaceAll(string(b), key, "LABKEY"))
		if err := os.WriteFile(filepath.Join("testdata", "sabnzbd", name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if record {
		_ = os.MkdirAll(filepath.Join("testdata", "sabnzbd"), 0o755)
		save("version.json", raw("version", url.Values{}))
		save("get_cats.json", raw("get_cats", url.Values{}))
	}

	cat := "movies"
	// Leftovers of an earlier run would be found by name and taken as these.
	if old, err := s.List(ctx, cat); err == nil {
		for _, j := range old {
			if j.Name == good || j.Name == bad {
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
		// The same name again is found, not added twice.
		again, err := s.Add(ctx, AddRequest{URL: "http://" + me + ":18799" + path, Category: cat, Name: name})
		if err != nil || again != id {
			t.Fatalf("second add gave %q %v, want %q", again, err, id)
		}
	}
	defer func() {
		for _, id := range ids {
			_ = s.Remove(context.Background(), id, true)
		}
	}()
	savedMoving := false
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if !savedMoving {
			if q := raw("queue", url.Values{"category": {cat}}); strings.Contains(string(q), `"status":"Downloading"`) {
				save("queue-downloading.json", q)
				savedMoving = true
			}
		}
		g, gerr := s.Get(ctx, ids[good])
		b, berr := s.Get(ctx, ids[bad])
		if gerr == nil && berr == nil && g.State == StateFinished && b.State == StateFailed {
			t.Logf("finished: %s in %s; failed: %s (%s)", g.Name, g.ContentPath, b.Name, b.Detail)
			list, err := s.List(ctx, cat)
			if err != nil || len(list) < 2 {
				t.Fatalf("list %v %v", list, err)
			}
			save("history.json", raw("history", url.Values{"category": {cat}, "limit": {"10"}}))
			for _, id := range ids {
				if err := s.Remove(ctx, id, true); err != nil {
					t.Fatalf("remove: %v", err)
				}
				if _, err := s.Get(ctx, id); err != ErrNotFound {
					t.Fatalf("still there after remove: %v", err)
				}
			}
			ids = nil
			if record {
				b, _ := json.Marshal(map[string]any{"articles_asked": nntp.asked})
				t.Logf("news server: %s", b)
			}
			t.Run("a broken NZB is replaced by the next Release", func(t *testing.T) { labReplace(t, base, key, "http://"+me+":18799") })
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	g, _ := s.Get(ctx, ids[good])
	b, _ := s.Get(ctx, ids[bad])
	t.Fatalf("not done in time: good %s (%s %.2f), bad %s (%s)", g.State, g.Detail, g.Progress, b.State, b.Detail)
}

func ownAddress(t *testing.T) string {
	t.Helper()
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	t.Fatal("no address on the lab network")
	return ""
}

const (
	labNext = "Lab.Usenet.Next.2024.1080p.WEB.H264-HOLYARRS"
	labDead = "Lab.Usenet.Dead.2024.1080p.WEB.H264-HOLYARRS"
)

// labReplace is roadmap 4.0.3 on the lab: the chosen NZB is broken, SABnzbd
// fails it, the manager turns it down and removes it, and the next Release
// (sent by OnFailed, standing in for the search) finishes.
func labReplace(t *testing.T, base, key, nzbBase string) {
	w, m := setup(t)
	ctx := context.Background()
	m.Now, m.Sleep = nil, nil
	m.FromVault = func(context.Context, int64) (string, error) { return key, nil }
	var ref int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO secrets (kind, value, created_at, created_by, updated_at, updated_by) VALUES ('downloader_secret', 'sealed', '', '{}', '', '{}')`)
		if err != nil {
			return err
		}
		ref, err = res.LastInsertId()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w.addDownloader(t, &domain.Downloader{Name: "Lab SABnzbd", Adapter: "sabnzbd", Protocol: "usenet", URL: base, Category: "movies", Priority: 1, AllowLocal: true, SecretRef: &ref})
	rels := map[string]int64{}
	for _, n := range []string{labDead, labNext} {
		link := nzbBase + "/dead.nzb"
		if n == labNext {
			link = nzbBase + "/next.nzb"
		}
		r := &domain.Release{SourceID: 1, Name: n, LinkRef: link, Protocol: "usenet", Size: 1, Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: []int64{w.slotID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "judged"}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := store.Insert(tx, "releases", r, domain.System); return err }); err != nil {
			t.Fatal(err)
		}
		rels[n] = r.ID
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", rels[labDead], w.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var next int64
	m.OnFailed = func(ctx context.Context, slotID int64) {
		if err := w.s.Write(ctx, func(tx *sql.Tx) error {
			if err := store.MoveSlotTx(tx, slotID, domain.SlotSearching, "searching", domain.System); err != nil {
				return err
			}
			if err := store.MoveSlotTx(tx, slotID, domain.SlotChosen, "chose the next", domain.System); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", rels[labNext], slotID)
			return err
		}); err != nil {
			t.Errorf("choose next Release: %v", err)
			return
		}
		id, err := m.Grab(ctx, slotID, rels[labNext], domain.System)
		if err != nil {
			t.Errorf("next grab: %v", err)
		}
		next = id
	}
	dead, err := m.Grab(ctx, w.slotID, rels[labDead], domain.System)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSabnzbd(base, key, nil)
	deadID := w.download(t, dead).ExternalID
	defer func() {
		_ = s.Remove(context.Background(), deadID, true)
		if next != 0 {
			_ = s.Remove(context.Background(), w.download(t, next).ExternalID, true)
		}
	}()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		_, _ = m.SyncOnce(ctx)
		if next != 0 && w.download(t, next).State == "finished" {
			d := w.download(t, dead)
			t.Logf("dead: %s (%s); next: finished with %d file(s)", d.State, *d.StateReason, len(w.download(t, next).FilesSeen))
			var st string
			_ = w.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", rels[labDead]).Scan(&st)
			if d.State != "failed" || st != "turned_down" {
				t.Fatalf("dead download %s, release %s", d.State, st)
			}
			if _, err := s.Get(ctx, deadID); err != ErrNotFound {
				t.Fatalf("the failed NZB is still in SABnzbd: %v", err)
			}
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("not replaced in time: dead %s, next %d", w.download(t, dead).State, next)
}
