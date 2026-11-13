package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const hash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"

var _ TorrentInspector = (*Qbittorrent)(nil)

var (
	_ Engine = (*Qbittorrent)(nil)
	_ Engine = (*Sabnzbd)(nil)
)

type fakeQbit struct {
	mu       sync.Mutex
	torrents map[string]map[string]any
	files    map[string][]map[string]any
	added    []map[string]string
	cats     []string
	deleted  []string
	refuse   bool
	key      string
	user     string
	pass     string
	signIns  int
	banned   bool
	old      bool
	prio     []string
	addFails bool
	selected []string
	// steps lists add, filePrio, start and delete calls in order; prioFails
	// makes filePrio answer 500.
	steps     []string
	prioFails bool
	// prioWait, set before the test starts, runs before each filePrio call
	// takes the lock, so a test can hold a call open.
	prioWait func()
}

func (f *fakeQbit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.prioWait != nil && r.URL.Path == "/api/v2/torrents/filePrio" {
		f.prioWait()
	}
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
			switch {
			case f.banned:
				w.WriteHeader(http.StatusForbidden)
			case r.PostForm.Get("username") == f.user && r.PostForm.Get("password") == f.pass:
				http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8090", Value: "lab-session", Path: "/"})
				if f.old {
					_, _ = w.Write([]byte("Ok."))
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			case f.old:
				_, _ = w.Write([]byte("Fails."))
			default:
				w.WriteHeader(http.StatusUnauthorized)
			}
			return
		}
		if c, err := r.Cookie("QBT_SID_8090"); err != nil || c.Value != "lab-session" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
	if f.key != "" {
		if r.Header.Get("Authorization") != "Bearer "+f.key || strings.HasPrefix(r.URL.Path, "/api/v2/auth/") {
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
		add := map[string]string{"urls": r.PostForm.Get("urls"), "category": r.PostForm.Get("category"), "savepath": r.PostForm.Get("savepath"),
			"stopped": r.PostForm.Get("stopped"), "paused": r.PostForm.Get("paused")}
		if r.ParseMultipartForm(1<<20) == nil {
			add["category"], add["savepath"] = r.FormValue("category"), r.FormValue("savepath")
			add["stopped"], add["paused"] = r.FormValue("stopped"), r.FormValue("paused")
			if file, _, err := r.FormFile("torrents"); err == nil {
				b, _ := io.ReadAll(file)
				add["torrent"] = string(b)
			}
		}
		f.added = append(f.added, add)
		f.steps = append(f.steps, "add stopped="+add["stopped"])
		_, _ = w.Write([]byte("Ok."))
	case "/api/v2/torrents/start":
		f.steps = append(f.steps, "start "+r.PostForm.Get("hashes"))
	case "/api/v2/torrents/stop":
		f.steps = append(f.steps, "stop "+r.PostForm.Get("hashes"))
	case "/api/v2/torrents/setCategory":
		f.steps = append(f.steps, "setCategory "+r.PostForm.Get("hashes")+" "+r.PostForm.Get("category"))
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
	case "/api/v2/torrents/properties":
		t := f.torrents[r.Form.Get("hash")]
		if t == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total_size": t["size"], "total_downloaded": 1000, "total_uploaded": 500, "total_wasted": 7,
			"dl_speed": t["dlspeed"], "up_speed": 20, "eta": 8640000, "share_ratio": 0.5, "seeds": 3, "seeds_total": 40, "peers": 2, "peers_total": 9,
			"addition_date": 1700000000, "completion_date": -1, "save_path": t["save_path"], "is_private": true})
	case "/api/v2/torrents/filePrio":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if f.prioFails {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		f.prio = append(f.prio, r.PostForm.Get("hash")+"|"+r.PostForm.Get("id")+"|"+r.PostForm.Get("priority"))
		f.steps = append(f.steps, "prio "+r.PostForm.Get("id")+"="+r.PostForm.Get("priority"))
	case "/api/v2/torrents/delete":
		f.steps = append(f.steps, "delete "+r.PostForm.Get("deleteFiles"))
		f.selected = append(f.selected, r.PostForm.Get("hashes"))
		f.deleted = append(f.deleted, r.PostForm.Get("hashes")+"|"+r.PostForm.Get("deleteFiles"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

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
		"error": StatePaused, "missingFiles": StatePaused, "unknown": StatePaused,
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

func TestQbitSendsKeyAsBearer(t *testing.T) {
	f, u := startFake(t)
	f.key = "lab-test-key"
	ctx := context.Background()
	q := NewQbittorrentKey(u, "lab-test-key", nil)
	if _, err := q.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.List(ctx, "holyarrs"); err != nil {
		t.Fatalf("list %v", err)
	}
	_, err := NewQbittorrentKey(u, "wrong", nil).Health(ctx)
	if !errors.Is(err, ErrAuth) || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := NewQbittorrent(u, nil).Health(ctx); !errors.Is(err, ErrAuth) {
		t.Fatalf("no key: %v", err)
	}
}

func TestQbitNeverSignsInWithPassword(t *testing.T) {
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Cookie") != "" {
			t.Errorf("cookie sent: %q", r.Header.Get("Cookie"))
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer s.Close()
	_, err := NewQbittorrentKey(s.URL, "k", nil).Health(context.Background())
	if !errors.Is(err, ErrAuth) || len(paths) != 1 || paths[0] != "/api/v2/app/version" {
		t.Fatalf("err %v paths %v (no sign-in retry)", err, paths)
	}
}

func TestQbitSignsInWithPassword(t *testing.T) {
	for _, old := range []bool{false, true} {
		f, u := startFake(t)
		f.user, f.pass, f.old = "holyarrs", "your-test-word", old
		ctx := context.Background()
		q := NewQbittorrentLogin(u, "holyarrs", "your-test-word", nil)
		if _, err := q.Health(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := q.List(ctx, "holyarrs"); err != nil || f.signIns != 1 {
			t.Fatalf("old=%v: list %v, sign-ins %d (the cookie must be kept)", old, err, f.signIns)
		}
	}
}

func TestQbitWrongPasswordIsNotRetriedAtOnce(t *testing.T) {
	f, u := startFake(t)
	f.user, f.pass = "holyarrs", "your-test-word"
	ctx := context.Background()
	q := NewQbittorrentLogin(u, "holyarrs", "wrong", nil)
	for i := 0; i < 4; i++ {
		_, err := q.Health(ctx)
		if !errors.Is(err, ErrAuth) || strings.Contains(err.Error(), "wrong") {
			t.Fatalf("wrong password: %v", err)
		}
	}
	if f.signIns != 1 {
		t.Fatalf("%d sign-ins; a refused one must wait (qBittorrent bans after 5)", f.signIns)
	}
	f.banned = true
	_, err := NewQbittorrentLogin(u, "holyarrs", "your-test-word", nil).Health(ctx)
	if !errors.Is(err, ErrQbitBanned) || !errors.Is(err, ErrAuth) {
		t.Fatalf("banned: %v", err)
	}
}

func TestQbitKeyRefusedSaysVersion(t *testing.T) {
	f, u := startFake(t)
	f.key = "k"
	_, err := NewQbittorrentKey(u, "other", nil).Health(context.Background())
	if !errors.Is(err, ErrQbitKeyRefused) || !strings.Contains(err.Error(), "5.2") {
		t.Fatalf("refused key: %v", err)
	}
}

func TestQbitPropertiesFilesAndPriority(t *testing.T) {
	f, u := startFake(t)
	f.key = "k"
	q := NewQbittorrentKey(u, "k", nil)
	ctx := context.Background()
	f.set(hash, map[string]any{"size": 4000, "dlspeed": 99, "save_path": "/downloads"})
	f.files[hash] = []map[string]any{{"index": 0, "name": "a/b.mkv", "size": 3000, "progress": 0.25, "priority": 1}, {"index": 1, "name": "a/c.nfo", "size": 1000, "progress": 0, "priority": 0}}
	p, err := q.Properties(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if p.Size != 4000 || p.Downloaded != 1000 || p.Speed != 99 || p.ETA != 0 || p.Ratio != 0.5 || p.SeedsTotal != 40 || p.PeersTotal != 9 ||
		p.AddedAt == nil || p.DoneAt != nil || p.SavePath != "/downloads" || p.Private == nil || !*p.Private || p.Wasted != 7 {
		t.Fatalf("properties %+v", p)
	}
	fs, err := q.TorrentFiles(ctx, hash)
	if err != nil || len(fs) != 2 || fs[1].Index != 1 || fs[1].Priority != 0 || fs[0].Name != "a/b.mkv" {
		t.Fatalf("files %+v %v", fs, err)
	}
	if err := q.SetFilePriority(ctx, hash, []int{0, 1}, 6); err != nil || len(f.prio) != 1 || f.prio[0] != hash+"|0|1|6" {
		t.Fatalf("prio %v %v", err, f.prio)
	}
	if err := q.SetFilePriority(ctx, hash, []int{0}, 3); !errors.Is(err, ErrFilePriority) {
		t.Fatalf("bad priority: %v", err)
	}
	if err := q.SetFilePriority(ctx, "all", []int{0}, 1); !errors.Is(err, errBadHash) {
		t.Fatalf("bad hash: %v", err)
	}
	if _, err := q.Properties(ctx, "ffffffffffffffffffffffffffffffffffffffff"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestQbitJobCarriesUploadSeedsAndPeers(t *testing.T) {
	j := qbitTorrent{Hash: "ABC", DLSpeed: 900, UPSpeed: 300, NumSeeds: 12, NumLeechs: 4, State: "downloading"}.job()
	if j.Speed != 900 || j.UpSpeed != 300 || j.Seeds != 12 || j.Peers != 4 {
		t.Fatalf("%+v", j)
	}
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

func startFake(t *testing.T) (*fakeQbit, string) {
	f := newFake()
	s := httptest.NewServer(f)
	t.Cleanup(s.Close)
	return f, s.URL
}
