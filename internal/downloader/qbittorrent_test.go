package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeQbit is a tiny stand-in for the qBittorrent Web API v2.
type fakeQbit struct {
	mu       sync.Mutex
	torrents map[string]map[string]any // by hash
	files    map[string][]map[string]any
	added    []map[string]string
	cats     []string
	deleted  []string
	refuse   bool   // answer 403 like a client that wants a login
	user     string // when set, a sign-in with user and pass is needed
	pass     string
	signIns  int
	addFails bool     // answer Fails. to an add
	selected []string // every hashes= and hash= value asked for
}

func newFake() *fakeQbit {
	return &fakeQbit{torrents: map[string]map[string]any{}, files: map[string][]map[string]any{}}
}

func (f *fakeQbit) set(hash string, fields map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := f.torrents[hash]
	if t == nil {
		t = map[string]any{"hash": hash}
		f.torrents[hash] = t
	}
	for k, v := range fields {
		t[k] = v
	}
}

func (f *fakeQbit) drop(hash string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.torrents, hash)
}

func (f *fakeQbit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	if f.user != "" {
		if r.URL.Path == "/api/v2/auth/login" {
			f.signIns++
			if r.PostForm.Get("username") == f.user && r.PostForm.Get("password") == f.pass {
				http.SetCookie(w, &http.Cookie{Name: "SID", Value: "lab-session", Path: "/"})
				_, _ = w.Write([]byte("Ok."))
				return
			}
			_, _ = w.Write([]byte("Fails."))
			return
		}
		if c, err := r.Cookie("SID"); err != nil || c.Value != "lab-session" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
	switch r.URL.Path {
	case "/api/v2/app/version":
		_, _ = w.Write([]byte("v5.2.4"))
	case "/api/v2/torrents/createCategory":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		for _, c := range f.cats {
			if c == r.PostForm.Get("category") {
				w.WriteHeader(http.StatusConflict)
				return
			}
		}
		f.cats = append(f.cats, r.PostForm.Get("category"))
	case "/api/v2/torrents/add":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if f.addFails {
			_, _ = w.Write([]byte("Fails."))
			return
		}
		f.added = append(f.added, map[string]string{"urls": r.PostForm.Get("urls"), "category": r.PostForm.Get("category"), "savepath": r.PostForm.Get("savepath")})
		_, _ = w.Write([]byte("Ok."))
	case "/api/v2/torrents/info":
		var out []map[string]any
		hashes := r.Form.Get("hashes")
		if hashes != "" {
			f.selected = append(f.selected, hashes)
		}
		for h, t := range f.torrents {
			if hashes != "" && !strings.Contains(hashes, h) {
				continue
			}
			if c := r.Form.Get("category"); c != "" && t["category"] != c {
				continue
			}
			out = append(out, t)
		}
		if out == nil {
			out = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(out)
	case "/api/v2/torrents/files":
		f.selected = append(f.selected, r.Form.Get("hash"))
		fs := f.files[r.Form.Get("hash")]
		if fs == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(fs)
	case "/api/v2/torrents/delete":
		f.selected = append(f.selected, r.PostForm.Get("hashes"))
		f.deleted = append(f.deleted, r.PostForm.Get("hashes")+"|"+r.PostForm.Get("deleteFiles"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func startFake(t *testing.T) (*fakeQbit, string) {
	f := newFake()
	s := httptest.NewServer(f)
	t.Cleanup(s.Close)
	return f, s.URL
}

const hash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"

func TestQbitHealthAddGetList(t *testing.T) {
	f, u := startFake(t)
	q := NewQbittorrent(u, nil)
	ctx := context.Background()
	h, err := q.Health(ctx)
	if err != nil || h.Version != "v5.2.4" {
		t.Fatalf("health %+v %v", h, err)
	}
	id, err := q.Add(ctx, AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: "holyarrs", SavePath: "/data/downloads/complete", InfoHash: strings.ToUpper(hash)})
	if err != nil || id != hash {
		t.Fatalf("add %q %v", id, err)
	}
	if len(f.added) != 1 || f.added[0]["category"] != "holyarrs" || f.added[0]["savepath"] != "/data/downloads/complete" || f.added[0]["urls"] != "http://fake-indexer/dune-part-two.torrent" {
		t.Fatalf("added %+v", f.added)
	}
	if len(f.cats) != 1 {
		t.Fatalf("category not created: %v", f.cats)
	}
	f.set(hash, map[string]any{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", "category": "holyarrs", "progress": 0.5,
		"dlspeed": 1000, "eta": 30, "state": "downloading", "save_path": "/data/downloads/complete", "size": 29588180,
		"content_path": "/data/downloads/complete/Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"})
	f.files[hash] = []map[string]any{{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", "size": 29588180, "progress": 0.5}}
	j, err := q.Get(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if j.State != StateDownloading || j.Progress != 0.5 || j.Speed != 1000 || j.ETA != 30 || len(j.Files) != 1 || j.Files[0].Size != 29588180 {
		t.Fatalf("job %+v", j)
	}
	list, err := q.List(ctx, "holyarrs")
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v %v", list, err)
	}
	if _, err := q.Get(ctx, "ffffffffffffffffffffffffffffffffffffffff"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing torrent: %v", err)
	}
	if err := q.Remove(ctx, hash, false); err != nil || len(f.deleted) != 1 || f.deleted[0] != hash+"|false" {
		t.Fatalf("remove %v %v", err, f.deleted)
	}
}

func TestQbitStates(t *testing.T) {
	cases := map[string]string{
		"downloading": StateDownloading, "stalledDL": StateDownloading, "metaDL": StateDownloading, "forcedDL": StateDownloading,
		"queuedDL": StateQueued, "checkingDL": StateQueued, "allocating": StateQueued,
		"uploading": StateFinished, "stalledUP": StateFinished, "pausedUP": StateFinished, "stoppedUP": StateFinished, "queuedUP": StateFinished,
		"pausedDL": StatePaused, "stoppedDL": StatePaused,
		"error": StateFailed, "missingFiles": StateFailed,
	}
	for in, want := range cases {
		if got := qbitState(in); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

func TestQbitLoginWantedIsAuthError(t *testing.T) {
	f, u := startFake(t)
	f.refuse = true
	_, err := NewQbittorrent(u, nil).Health(context.Background())
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("want ErrAuth, got %v", err)
	}
}

func TestQbitSignsInWhenAsked(t *testing.T) {
	f, u := startFake(t)
	f.user, f.pass = "holyarrs", "lab-test-pw"
	ctx := context.Background()
	q := NewQbittorrentLogin(u, "holyarrs", "lab-test-pw", nil)
	if _, err := q.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.List(ctx, "holyarrs"); err != nil || f.signIns != 1 {
		t.Fatalf("list %v, sign-ins %d (the cookie must be kept)", err, f.signIns)
	}
	_, err := NewQbittorrentLogin(u, "holyarrs", "wrong", nil).Health(ctx)
	if !errors.Is(err, ErrAuth) || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("wrong password: %v", err)
	}
}

// A third engine only needs the Engine methods.
var (
	_ Engine = (*Qbittorrent)(nil)
	_ Engine = (*Sabnzbd)(nil)
)
