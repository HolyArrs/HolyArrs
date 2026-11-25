package downloads

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type sabPaged struct {
	mu        sync.Mutex
	queue     int
	history   int
	counter   int64
	calls     []string
	retryID   string
	retried   []string
	unchanged bool
}

func TestSabQueueReadsEveryPage(t *testing.T) {
	f := &sabPaged{queue: 450}
	s := NewSabnzbd(f.serve(t), sabKey, nil)
	p, err := s.Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Jobs) != 450 {
		t.Fatalf("%d jobs", len(p.Jobs))
	}
	if n := len(f.calls); n != 3 {
		t.Fatalf("%d calls: %v", n, f.calls)
	}
	if p.Head.Speed != 2048*1024 || p.Head.TimeLeft != 3600 || p.Head.FreeSpace != 4*1024*1024*1024 {
		t.Fatalf("head %+v", p.Head)
	}
	if j := p.Jobs[1]; j.State != StatePaused || j.QueuePos != 2 || j.AddedAt != 1700000001 {
		t.Fatalf("job %+v", j)
	}
	var total int64
	for _, j := range p.Jobs {
		total += j.Speed
	}
	if total != 2048*1024 || p.Jobs[0].Speed != total {
		t.Fatalf("lead job speed %d, total %d", p.Jobs[0].Speed, total)
	}
}

// SABnzbd reports every unpaused slot as Downloading and one speed for the
// whole queue, so each job's share comes from how fast its bytes left go down:
// the first job alone at first, then the next one too as the first finishes.
func TestSabSpeedFollowsMeasuredProgress(t *testing.T) {
	type slot struct {
		id, cat, status string
		left            float64 // MB
	}
	var mu sync.Mutex
	queue := []slot{{"p", "tv", "Paused", 100}, {"a", "movies", "Downloading", 50}, {"b", "tv", "Downloading", 100}, {"c", "tv", "Downloading", 100}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		q := r.URL.Query()
		if q.Get("mode") == "history" {
			_, _ = w.Write([]byte(`{"history":{"slots":[]}}`))
			return
		}
		var out []string
		for _, x := range queue {
			if c := q.Get("category"); c != "" && c != x.cat {
				continue
			}
			if id := q.Get("nzo_ids"); id != "" && id != x.id {
				continue
			}
			out = append(out, fmt.Sprintf(`{"nzo_id":%q,"filename":%q,"cat":%q,"mb":"100","mbleft":"%.2f","status":%q,"timeleft":"0:01:00"}`, x.id, x.id, x.cat, x.left, x.status))
		}
		fmt.Fprintf(w, `{"queue":{"kbpersec":"10240","noofslots_total":%d,"slots":[%s]}}`, len(out), strings.Join(out, ","))
	}))
	t.Cleanup(srv.Close)
	at := time.Unix(1700000000, 0)
	s := NewSabnzbd(srv.URL, sabKey, nil)
	s.now = func() time.Time { return at }
	move := func(d time.Duration, moved map[string]float64) {
		mu.Lock()
		defer mu.Unlock()
		at = at.Add(d)
		for i := range queue {
			queue[i].left -= moved[queue[i].id]
		}
	}
	read := func() map[string]int64 {
		t.Helper()
		p, err := s.Queue(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]int64{}
		for _, j := range p.Jobs {
			m[j.ID] = j.Speed
		}
		return m
	}
	const full, mb = 10240 * 1024, 1024 * 1024
	check := func(what string, got map[string]int64, want map[string]int64) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: speeds %v, want %v", what, got, want)
		}
	}
	// Nothing measured yet: the first Downloading job in queue order.
	check("first read", read(), map[string]int64{"p": 0, "a": full, "b": 0, "c": 0})
	// Only a moved.
	move(2*time.Second, map[string]float64{"a": 20})
	check("a alone", read(), map[string]int64{"p": 0, "a": full, "b": 0, "c": 0})
	// a is finishing and b has started: a quarter and three quarters.
	move(2*time.Second, map[string]float64{"a": 5, "b": 15})
	check("overlap", read(), map[string]int64{"p": 0, "a": full / 4, "b": full * 3 / 4, "c": 0})
	// A read sooner than the gap keeps the last rates.
	move(200*time.Millisecond, nil)
	check("quick read", read(), map[string]int64{"p": 0, "a": full / 4, "b": full * 3 / 4, "c": 0})
	// A filtered list gives each job its own measured rate, not a share of
	// the total: b moves 4 MB in the 2 s since its last measure.
	move(1800*time.Millisecond, map[string]float64{"b": 4})
	jobs, err := s.List(context.Background(), "tv")
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if want := map[string]int64{"b": 2 * mb}[j.ID]; j.Speed != want {
			t.Fatalf("list tv: %s speed %d, want %d", j.ID, j.Speed, want)
		}
	}
	// A filtered list with nothing measured shows no speed rather than guess.
	fresh := NewSabnzbd(srv.URL, sabKey, nil)
	if j, err := fresh.Get(context.Background(), "a"); err != nil || j.Speed != 0 {
		t.Fatalf("fresh get a: %+v %v", j, err)
	}
}

func TestSabHistoryPagesAndFastPath(t *testing.T) {
	f := &sabPaged{history: 430, counter: 7}
	s := NewSabnzbd(f.serve(t), sabKey, nil)
	p, err := s.History(context.Background(), 0)
	if err != nil || !p.Changed || p.Last != 7 || len(p.Jobs) != 430 {
		t.Fatalf("%v changed=%v last=%d jobs=%d", err, p.Changed, p.Last, len(p.Jobs))
	}
	if p.Jobs[1].State != StateFailed || p.Jobs[0].State != StateFinished || p.Jobs[0].DoneAt != 1700001000 {
		t.Fatalf("%+v %+v", p.Jobs[0], p.Jobs[1])
	}
	for _, unchanged := range []bool{false, true} {
		f.unchanged = unchanged
		f.calls = nil
		p, err = s.History(context.Background(), 7)
		if err != nil || p.Changed || len(p.Jobs) != 0 || p.Last != 7 || len(f.calls) != 1 {
			t.Fatalf("unchanged=%v: %v %+v calls %v", unchanged, err, p, f.calls)
		}
	}
	f.counter = 2
	f.calls = nil
	p, err = s.History(context.Background(), 7)
	if err != nil || !p.Changed || p.Last != 2 || len(p.Jobs) != 430 {
		t.Fatalf("reset: %v changed=%v last=%d jobs=%d", err, p.Changed, p.Last, len(p.Jobs))
	}
	if !strings.HasSuffix(f.calls[0], ":7") || strings.HasSuffix(f.calls[1], ":7") {
		t.Fatalf("later pages must not carry the counter: %v", f.calls)
	}
}

func TestSabRetryNewID(t *testing.T) {
	f := &sabPaged{retryID: "SABnzbd_nzo_new"}
	s := NewSabnzbd(f.serve(t), sabKey, nil)
	id, err := s.Retry(context.Background(), "SABnzbd_nzo_old")
	if err != nil || id != "SABnzbd_nzo_new" || f.retried[0] != "SABnzbd_nzo_old" {
		t.Fatalf("%v %q %v", err, id, f.retried)
	}
	f.retryID = ""
	if id, err = s.Retry(context.Background(), "SABnzbd_nzo_same"); err != nil || id != "SABnzbd_nzo_same" {
		t.Fatalf("same id: %v %q", err, id)
	}
	if _, err = s.Retry(context.Background(), "a,b"); err != ErrNotFound {
		t.Fatal("a list of ids must be refused")
	}
}

func TestSabServerStats(t *testing.T) {
	s := NewSabnzbd((&sabPaged{}).serve(t), sabKey, nil)
	st, err := s.ServerStats(context.Background())
	if err != nil || st.Total != 300 || st.Week != 100 || len(st.Servers) != 2 {
		t.Fatalf("%v %+v", err, st)
	}
	if st.Servers[0].Name != "big.example" || st.Servers[1].ArticlesFound != 9 || st.Servers[1].ArticlesTried != 10 || st.Servers[0].ArticlesTried != 40 || st.Servers[0].ArticlesFound != 38 {
		t.Fatalf("%+v", st.Servers)
	}
}

func TestSabJobStagesRedacted(t *testing.T) {
	s := NewSabnzbd((&sabPaged{counter: 1}).serve(t), sabKey, nil)
	st, err := s.JobStages(context.Background(), "SABnzbd_nzo_x")
	if err != nil || len(st) != 5 {
		t.Fatalf("%v %+v", err, st)
	}
	want := []string{"source", "servers", "repair", "unpack", "script"}
	for i, w := range want {
		if st[i].Stage != w {
			t.Fatalf("stage %d = %s", i, st[i].Stage)
		}
	}
	if l := st[0].Lines[0]; strings.Contains(l, "SECRETKEY123") || strings.Contains(l, "apikey") {
		t.Fatalf("source line not redacted: %q", l)
	}
	if l := st[3].Lines[0]; strings.Contains(l, "<") {
		t.Fatalf("markup kept: %q", l)
	}
	if _, err := s.JobStages(context.Background(), ""); err != ErrNotFound {
		t.Fatal("empty id")
	}
}

func TestSabMoveTopAlreadyFirst(t *testing.T) {
	s := NewSabnzbd((&sabPaged{queue: 1}).serve(t), sabKey, nil)
	if err := s.MoveTop(context.Background(), "SABnzbd_nzo_q0"); err != nil {
		t.Fatalf("only job: %v", err)
	}
	if err := s.MoveTop(context.Background(), "SABnzbd_nzo_gone"); err != ErrNotFound {
		t.Fatalf("missing job: %v", err)
	}
}

func TestCleanLineMasksPassword(t *testing.T) {
	got := cleanLine(`[x] Trying unrar with password "hunter2"`)
	if strings.Contains(got, "hunter2") || !strings.Contains(got, "Trying unrar with password") {
		t.Fatalf("got %q", got)
	}
}

func (f *sabPaged) serve(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		q := r.URL.Query()
		if q.Get("apikey") != sabKey {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("API Key Incorrect"))
			return
		}
		start, _ := strconv.Atoi(q.Get("start"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		f.calls = append(f.calls, fmt.Sprintf("%s:%d:%s", q.Get("mode"), start, q.Get("last_history_update")))
		switch q.Get("mode") {
		case "queue":
			var slots []string
			for i := start; i < min(start+limit, f.queue); i++ {
				st := "Downloading"
				if i%3 == 1 {
					st = "Paused"
				}
				slots = append(slots, fmt.Sprintf(`{"index":%d,"nzo_id":"SABnzbd_nzo_q%d","filename":"Show.S01E%02d.1080p","cat":"tv","mb":"100","mbleft":"50","percentage":"50","status":"%s","timeleft":"0:01:00","time_added":%d}`, i, i, i+1, st, 1700000000+i))
			}
			fmt.Fprintf(w, `{"queue":{"paused":false,"kbpersec":"2048","timeleft":"1:00:00","diskspace1":"10","diskspace2":"4","noofslots_total":%d,"slots":[%s]}}`, f.queue, strings.Join(slots, ","))
		case "history":
			if ids := q.Get("nzo_ids"); ids != "" {
				fmt.Fprintf(w, `{"history":{"noofslots":1,"last_history_update":%d,"slots":[{"nzo_id":%q,"name":"Movie.2024","status":"Completed","bytes":10,"stage_log":[{"name":"Source","actions":["https://indexer.example/get?apikey=SECRETKEY123&id=1"]},{"name":"Servers","actions":["news.example=1.2 GB"]},{"name":"Repair","actions":["[Movie] Verified in 2 seconds, all files correct"]},{"name":"Unpack","actions":["[Movie] Unpacked 1 files/folders in 3 seconds<br/>"]},{"name":"Script","actions":["done.py: ok"]}]}]}}`, f.counter, ids)
				return
			}
			since, _ := strconv.ParseInt(q.Get("last_history_update"), 10, 64)
			if since > 0 && since == f.counter {
				if f.unchanged {
					_, _ = w.Write([]byte(`{"history":false}`))
				} else {
					fmt.Fprintf(w, `{"history":{"noofslots":%d,"last_history_update":%d,"slots":[]}}`, f.history, f.counter)
				}
				return
			}
			var slots []string
			for i := start; i < min(start+limit, f.history); i++ {
				st := "Completed"
				if i%2 == 1 {
					st = "Failed"
				}
				slots = append(slots, fmt.Sprintf(`{"nzo_id":"SABnzbd_nzo_h%d","name":"Movie.%d.2024","category":"movies","bytes":5,"status":"%s","storage":"/d/m/%d","completed":%d,"time_added":%d}`, i, i, st, i, 1700001000+i, 1700000000+i))
			}
			fmt.Fprintf(w, `{"history":{"noofslots":%d,"last_history_update":%d,"slots":[%s]}}`, f.history, f.counter, strings.Join(slots, ","))
		case "retry":
			f.retried = append(f.retried, q.Get("value"))
			if f.retryID == "" {
				_, _ = w.Write([]byte(`{"status":true}`))
				return
			}
			fmt.Fprintf(w, `{"status":true,"nzo_id":%q}`, f.retryID)
		case "switch":
			_, _ = w.Write([]byte(`{"result":{"position":-1,"priority":0}}`))
		case "server_stats":
			_, _ = w.Write([]byte(`{"total":300,"month":200,"week":100,"day":10,"servers":{"small.example":{"total":100,"month":50,"week":20,"day":1,"articles_tried":10,"articles_success":9,"daily":{"2026-10-30":1}},"big.example":{"total":200,"month":150,"week":80,"day":9,"articles_tried":{"2026-10-30":30,"2026-10-31":10},"articles_success":{"2026-10-31":38}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
