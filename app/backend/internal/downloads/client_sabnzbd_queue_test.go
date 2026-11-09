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
	if total > 2048*1024 || total < 2048*1024-450 {
		t.Fatalf("shared speed sums to %d", total)
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
