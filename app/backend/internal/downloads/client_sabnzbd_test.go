package downloads

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const sabKey = "0123456789abcdef0123456789abcdef"

const (
	sabGood = "ed7d10cb-a608-4f83-92f0-f78e38f07a0e"
	sabBad  = "6d1eae1a-3a5b-44ed-af8c-8fcee4d43d6b"
)

type fakeSab struct {
	mu      sync.Mutex
	queue   string
	history string
	calls   []string
	deleted []string
	added   []map[string]string
	refuse  string
	addFail bool
}

func TestSabnzbdHealthChecksTheKey(t *testing.T) {
	f := &fakeSab{}
	u := startSab(t, f)
	h, err := NewSabnzbd(u, sabKey, nil).Health(context.Background())
	if err != nil || h.Version != "5.1.3" {
		t.Fatalf("health %+v %v", h, err)
	}
	if _, err := NewSabnzbd(u, "wrong", nil).Health(context.Background()); !errors.Is(err, ErrAuth) {
		t.Fatalf("wrong key: %v", err)
	}
	f.refuse = "Access denied - Hostname verification failed: https://sabnzbd.org/hostname-check"
	_, err = NewSabnzbd(u, sabKey, nil).Health(context.Background())
	if err == nil || errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "Hostname verification") {
		t.Fatalf("host name refusal: %v", err)
	}
}

func TestSabnzbdQueueStates(t *testing.T) {
	f := &fakeSab{queue: "queue-downloading.json"}
	s := NewSabnzbd(startSab(t, f), sabKey, nil)
	js, err := s.List(context.Background(), "movies")
	if err != nil || len(js) != 1 {
		t.Fatalf("list %+v %v", js, err)
	}
	j := js[0]
	if j.ID != sabGood || j.Name != "Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS" || j.State != StateDownloading || j.Size != 1237319 || j.Progress != 0 {
		t.Fatalf("downloading row %+v", j)
	}
	for word, want := range map[string]string{"Grabbing": StateQueued, "Queued": StateQueued, "Propagating": StateQueued, "Fetching": StateQueued,
		"Checking": StateQueued, "Paused": StatePaused, "Downloading": StateDownloading, "SomethingNew": StateDownloading} {
		if got := sabQueueState(word); got != want {
			t.Errorf("%s -> %s, want %s", word, got, want)
		}
	}
}

func TestSabnzbdHistoryFinishedAndFailed(t *testing.T) {
	f := &fakeSab{history: "history.json"}
	s := NewSabnzbd(startSab(t, f), sabKey, nil)
	g, err := s.Get(context.Background(), sabGood)
	if err != nil {
		t.Fatal(err)
	}
	if g.State != StateFinished || g.Progress != 1 || g.SavePath != "/data/downloads/usenet/complete/movies/Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS.1" ||
		!strings.HasSuffix(g.ContentPath, ".mkv") || g.Size != 1238952 {
		t.Fatalf("finished %+v", g)
	}
	b, err := s.Get(context.Background(), sabBad)
	if err != nil || b.State != StateFailed || !strings.Contains(b.Detail, "Not on your server") {
		t.Fatalf("failed %+v %v", b, err)
	}
	if _, err := s.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestSabnzbdAddOnceByName(t *testing.T) {
	f := &fakeSab{}
	s := NewSabnzbd(startSab(t, f), sabKey, nil)
	ctx := context.Background()
	id, err := s.Add(ctx, AddRequest{URL: "http://lab/good.nzb", Category: "movies", Name: "Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS"})
	if err != nil || id != sabGood || len(f.added) != 1 || f.added[0]["cat"] != "movies" || f.added[0]["nzbname"] == "" {
		t.Fatalf("add %q %v %+v", id, err, f.added)
	}
	f.history = "history.json"
	id, err = s.Add(ctx, AddRequest{URL: "http://lab/good.nzb", Category: "movies", Name: "lab.usenet.movie.2024.1080p.web.h264-holyarrs"})
	if err != nil || id != sabGood || len(f.added) != 1 {
		t.Fatalf("second add %q %v %d", id, err, len(f.added))
	}
	if _, err := s.Add(ctx, AddRequest{URL: "http://lab/bad.nzb", Category: "movies", Name: "Lab.Usenet.Broken.2024.1080p.WEB.H264-HOLYARRS"}); err != nil || len(f.added) != 2 {
		t.Fatalf("add after failure %v %d", err, len(f.added))
	}
	if _, err := s.Add(ctx, AddRequest{URL: "http://lab/x.nzb", Category: "holyarrs", Name: "x"}); !errors.Is(err, ErrNoCategory) {
		t.Fatalf("missing category: %v", err)
	}
	f.addFail = true
	if _, err := s.Add(ctx, AddRequest{URL: "http://lab/y.nzb", Category: "movies", Name: "y"}); err == nil {
		t.Fatal("refused add gave no error")
	}
}

func TestSabnzbdRemoveFromBothLists(t *testing.T) {
	f := &fakeSab{}
	s := NewSabnzbd(startSab(t, f), sabKey, nil)
	if err := s.Remove(context.Background(), sabBad, true); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 2 || f.deleted[0] != "queue:"+sabBad+":1" || f.deleted[1] != "history:"+sabBad+":1" {
		t.Fatalf("deleted %v", f.deleted)
	}
}

func TestSabnzbdErrorsNeverCarryTheKey(t *testing.T) {
	s := NewSabnzbd("http://127.0.0.1:1", sabKey, nil)
	_, err := s.Health(context.Background())
	if err == nil {
		t.Fatal("no error from a closed port")
	}
	_, err2 := s.List(context.Background(), "movies")
	for _, e := range []error{err, err2} {
		if e == nil || strings.Contains(e.Error(), sabKey) || strings.Contains(e.Error(), "apikey") {
			t.Fatalf("error carries the key: %v", e)
		}
	}
}

func TestSabnzbdClock(t *testing.T) {
	for in, want := range map[string]int64{"0:04:10": 250, "1:02:00:05": 93605, "": 0, "x": 0, "0:00:00": 0} {
		if got := clock(in); got != want {
			t.Errorf("clock(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSabnzbdFailReasonPrefersStageLog(t *testing.T) {
	var x sabHistorySlot
	x.Status, x.FailMessage = "Failed", "Aborted, cannot be completed - https://sabnzbd.org/not-complete"
	x.StageLog = append(x.StageLog, struct {
		Name    string   `json:"name"`
		Actions []string `json:"actions"`
	}{"Download", []string{"Downloaded in 3 minutes"}}, struct {
		Name    string   `json:"name"`
		Actions []string `json:"actions"`
	}{"Repair", []string{"136 articles were missing", ""}})
	if d := x.job().Detail; d != "136 articles were missing" {
		t.Fatalf("detail %q", d)
	}
	x.StageLog = nil
	if d := x.job().Detail; d != "Aborted, cannot be completed" {
		t.Fatalf("no log: %q", d)
	}
	x.FailMessage = "Download failed - Not on your server(s) - https://sabnzbd.org/not-complete"
	if d := x.job().Detail; d != "Download failed - Not on your server(s)" {
		t.Fatalf("own: %q", d)
	}
}

func recorded(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "sabnzbd", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func startSab(t *testing.T, f *fakeSab) string {
	t.Helper()
	files := map[string]string{}
	for _, n := range []string{"version.json", "get_cats.json", "queue-downloading.json", "history.json"} {
		files[n] = recorded(t, n)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		q := r.URL.Query()
		mode := q.Get("mode")
		f.calls = append(f.calls, mode+":"+q.Get("name"))
		if r.URL.Path != "/api" {
			http.NotFound(w, r)
			return
		}
		if f.refuse != "" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(f.refuse))
			return
		}
		if mode != "version" && q.Get("apikey") != sabKey {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("API Key Incorrect"))
			return
		}
		switch {
		case mode == "version":
			_, _ = w.Write([]byte(files["version.json"]))
		case mode == "get_cats":
			_, _ = w.Write([]byte(files["get_cats.json"]))
		case mode == "addurl":
			if f.addFail {
				_, _ = w.Write([]byte(`{"status": false, "error": "expects one parameter"}`))
				return
			}
			f.added = append(f.added, map[string]string{"name": q.Get("name"), "cat": q.Get("cat"), "nzbname": q.Get("nzbname")})
			_, _ = w.Write([]byte(`{"status": true, "nzo_ids": ["ed7d10cb-a608-4f83-92f0-f78e38f07a0e"]}`))
		case (mode == "queue" || mode == "history") && q.Get("name") == "delete":
			f.deleted = append(f.deleted, mode+":"+q.Get("value")+":"+q.Get("del_files"))
			_, _ = w.Write([]byte(`{"status": true, "nzo_ids": ["` + q.Get("value") + `"]}`))
		case (mode == "queue" || mode == "history") && q.Get("category") != "" && q.Get("category") != "movies":
			_, _ = w.Write([]byte(`{"queue": {"status": "Idle", "kbpersec": "0.00", "slots": []}, "history": {"slots": []}}`))
		case mode == "queue":
			if f.queue == "" {
				_, _ = w.Write([]byte(`{"queue": {"status": "Idle", "kbpersec": "0.00", "slots": []}}`))
				return
			}
			_, _ = w.Write([]byte(files[f.queue]))
		case mode == "history":
			if f.history == "" {
				_, _ = w.Write([]byte(`{"history": {"slots": []}}`))
				return
			}
			_, _ = w.Write([]byte(files[f.history]))
		default:
			_, _ = w.Write([]byte(`{"status": false, "error": "not implemented"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
