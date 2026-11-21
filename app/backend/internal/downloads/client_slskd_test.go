package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

const slskdKey = "slskd-lab-key"

// fakeSlskd is a stand-in slskd: its download list, the enqueues it took and
// the transfers removed. readonly makes the key one that may only read.
type fakeSlskd struct {
	mu       sync.Mutex
	files    []slskdTransfer
	enqueued int
	removed  []string
	readonly bool
	failAll  bool
	options  string
	plain    bool
	// adminOnly answers the settings as slskd 0.26 does for a readwrite key.
	adminOnly bool
}

func startSlskd(t *testing.T, f *fakeSlskd) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Kestrel")
		if r.Header.Get("X-API-Key") != slskdKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		p := r.URL.EscapedPath()
		switch {
		case r.Method == http.MethodGet && p == "/api/v0/application/version":
			if f.plain {
				_, _ = w.Write([]byte("0.22.5"))
				return
			}
			_, _ = w.Write([]byte(`"0.22.5"`))
		case r.Method == http.MethodGet && p == "/api/v0/application":
			_, _ = w.Write([]byte(`{"server":{"state":"Connected, LoggedIn"}}`))
		case r.Method == http.MethodGet && p == "/api/v0/options":
			if f.adminOnly {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			o := f.options
			if o == "" {
				o = `{"directories":{"downloads":"/app/downloads"},"transfers":{"download":{"destination":{"subdirectory":"${SOURCE_DIRECTORY}"}}}}`
			}
			_, _ = w.Write([]byte(o))
		case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/v0/transfers/downloads/"):
			if f.readonly {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			user := strings.TrimPrefix(r.URL.Path, "/api/v0/transfers/downloads/")
			var in []struct {
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			}
			if json.NewDecoder(r.Body).Decode(&in) != nil || len(in) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("At least one file is required"))
				return
			}
			if f.failAll {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"enqueued":[],"failed":[{"filename":"x","message":"Skipped: user is offline"}]}`))
				return
			}
			f.enqueued++
			for i, x := range in {
				f.files = append(f.files, slskdTransfer{ID: user + "-" + string(rune('a'+i)), Username: user, Filename: x.Filename, Size: x.Size,
					State: "Queued, Remotely", RequestedAt: "2026-11-21T10:00:00.1234567Z"})
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"enqueued":[{}],"failed":[]}`))
		case r.Method == http.MethodGet && p == "/api/v0/transfers/downloads":
			type dir struct {
				Directory string          `json:"directory"`
				Files     []slskdTransfer `json:"files"`
			}
			type user struct {
				Username    string `json:"username"`
				Directories []dir  `json:"directories"`
			}
			var out []user
			for _, x := range f.files {
				d := x.Filename[:strings.LastIndex(x.Filename, `\`)]
				ui := -1
				for i := range out {
					if out[i].Username == x.Username {
						ui = i
					}
				}
				if ui < 0 {
					out = append(out, user{Username: x.Username})
					ui = len(out) - 1
				}
				di := -1
				for i := range out[ui].Directories {
					if out[ui].Directories[i].Directory == d {
						di = i
					}
				}
				if di < 0 {
					out[ui].Directories = append(out[ui].Directories, dir{Directory: d})
					di = len(out[ui].Directories) - 1
				}
				out[ui].Directories[di].Files = append(out[ui].Directories[di].Files, x)
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodDelete && strings.HasPrefix(p, "/api/v0/transfers/downloads/"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			for i, x := range f.files {
				if x.ID == id {
					f.files = append(f.files[:i], f.files[i+1:]...)
					f.removed = append(f.removed, id)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *fakeSlskd) setState(state string, done int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.files {
		f.files[i].State, f.files[i].BytesTransferred = state, min(done, f.files[i].Size)
	}
}

var slskdLabRelease = sources.SlskdRelease{User: "lab user", Files: []sources.SlskdFile{
	{Name: `@@music\Lab Artist\Lab Album (2024)\01 - One.flac`, Size: 30 << 20},
	{Name: `@@music\Lab Artist\Lab Album (2024)\02 - Two.flac`, Size: 20 << 20},
}}

func slskdHarness(t *testing.T) conformance {
	f := &fakeSlskd{}
	u := startSlskd(t, f)
	at := func(base string) Engine { e := NewSlskd(base, slskdKey, nil); e.Category = "holyarrs"; return e }
	bad := NewSlskd(u, "wrong", nil)
	return conformance{
		name:    "slskd",
		engine:  at(u),
		badKey:  bad,
		at:      at,
		secret:  slskdKey,
		release: AddRequest{URL: sources.SlskdLink(slskdLabRelease), Name: "Lab Artist - Lab Album (2024) [FLAC]", Category: "holyarrs"},
		unknown: "slskd-00000000000000000000",
		appear:  func() { f.setState("InProgress", 10<<20) },
		adds:    func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.enqueued },
		states:  func(w string) { f.setState(w, 50<<20) },
		words: map[string]string{"Queued, Remotely": StateQueued, "Requested": StateQueued, "Initializing": StateDownloading, "InProgress": StateDownloading,
			"Completed, Succeeded": StateFinished, "Completed, Errored": StateFailed, "Completed, Rejected": StateFailed, "Completed, Cancelled": StateFailed},
	}
}

func TestSlskdConformance(t *testing.T) { runConformance(t, slskdHarness(t)) }

func TestSlskdJobFolder(t *testing.T) {
	f := &fakeSlskd{}
	e := NewSlskd(startSlskd(t, f), slskdKey, nil)
	e.Category = "holyarrs"
	ctx := context.Background()
	id, err := e.Add(ctx, AddRequest{URL: sources.SlskdLink(slskdLabRelease)})
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := slskdLinkID(sources.SlskdLink(slskdLabRelease)); id != want {
		t.Fatalf("id %s, the journal would look for %s", id, want)
	}
	f.setState("Completed, Succeeded", 1<<40)
	j, err := e.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if j.State != StateFinished || j.Progress != 1 || j.Size != 50<<20 || j.Name != "Lab Album (2024)" {
		t.Errorf("job %+v", j)
	}
	// slskd saves a folder under its last name in its download folder.
	if j.ContentPath != "/app/downloads/Lab Album (2024)" || j.SavePath != "/app/downloads" {
		t.Errorf("content path %q in %q", j.ContentPath, j.SavePath)
	}
	if len(j.Files) != 2 || j.Files[0].Name != "Lab Album (2024)/01 - One.flac" {
		t.Errorf("files %+v", j.Files)
	}
	if jobs, _ := e.List(ctx, "another"); len(jobs) != 0 {
		t.Errorf("another category listed %d", len(jobs))
	}
	// A lost reply is looked for by the id from the link, never sent again.
	got, err := e.Add(ctx, AddRequest{ReconcileOnly: true, InfoHash: id})
	if err != nil || got != id || f.enqueued != 1 {
		t.Errorf("reconcile: %q %v, %d sends", got, err, f.enqueued)
	}
	if err := e.Remove(ctx, id, true); err != nil || len(f.removed) != 2 {
		t.Errorf("remove: %v, removed %v", err, f.removed)
	}
	if _, err := e.Add(ctx, AddRequest{ReconcileOnly: true, InfoHash: id}); !errors.Is(err, ErrNotArrived) || !errors.Is(err, ErrUncertain) {
		t.Errorf("reconcile after remove: %v", err)
	}
}

func TestSlskdStates(t *testing.T) {
	retry := "2026-11-21T10:05:00Z"
	for _, c := range []struct {
		states []string
		next   *string
		want   string
	}{
		{[]string{"Completed, Succeeded", "InProgress"}, nil, StateDownloading},
		{[]string{"Completed, Succeeded", "Completed, Errored"}, nil, StateFailed},
		{[]string{"Completed, Succeeded", "Completed, Errored"}, &retry, StateQueued},
		{[]string{"Completed, Succeeded", "Queued, Remotely"}, nil, StateQueued},
		{[]string{"Completed, TimedOut", "Queued, Locally"}, nil, StateQueued},
	} {
		var ts []slskdTransfer
		for _, s := range c.states {
			ts = append(ts, slskdTransfer{Filename: `a\b\c.flac`, Size: 10, State: s, NextAttemptAt: c.next})
		}
		if j := slskdJob("u", `a\b`, ts, "/d", "holyarrs"); j.State != c.want || j.Detail == "" {
			t.Errorf("%v: %s %q, want %s", c.states, j.State, j.Detail, c.want)
		}
	}
}

func TestSlskdHealth(t *testing.T) {
	ctx := context.Background()
	f := &fakeSlskd{}
	u := startSlskd(t, f)
	h, err := NewSlskd(u, slskdKey, nil).Health(ctx)
	if err != nil || h.Version != "0.22.5" || h.Note != "" {
		t.Fatalf("health %+v %v", h, err)
	}
	f.plain = true
	if h, err := NewSlskd(u, slskdKey, nil).Health(ctx); err != nil || h.Version != "0.22.5" {
		t.Errorf("plain-text version: %+v %v", h, err)
	}
	f.readonly = true
	if _, err := NewSlskd(u, slskdKey, nil).Health(ctx); err == nil || !strings.Contains(err.Error(), "readwrite") {
		t.Errorf("read-only key: %v", err)
	}
	f.readonly = false
	f.options = `{"directories":{"downloads":"/app/downloads"},"transfers":{"download":{"destination":{"subdirectory":"${SOURCE_USERNAME}"}}}}`
	if h, err := NewSlskd(u, slskdKey, nil).Health(ctx); err != nil || !strings.Contains(h.Note, "SOURCE_USERNAME") {
		t.Errorf("other folder pattern: %+v %v", h, err)
	}
	f.options = ""
	f.adminOnly = true
	if _, err := NewSlskd(u, slskdKey, nil).Health(ctx); err == nil || !strings.Contains(err.Error(), "path pair") {
		t.Errorf("settings hidden, no path pair: %v", err)
	}
	e := NewSlskd(u, slskdKey, nil)
	e.Folder = "/app/downloads/"
	if h, err := e.Health(ctx); err != nil || h.Version == "" {
		t.Errorf("settings hidden, path pair: %+v %v", h, err)
	}
	if dir, err := e.saveDir(ctx); err != nil || dir != "/app/downloads" {
		t.Errorf("save folder %q %v", dir, err)
	}
	f.adminOnly = false
	f.options = `{"directories":{}}`
	if _, err := NewSlskd(u, slskdKey, nil).Health(ctx); err == nil {
		t.Error("no download folder passed")
	}
}

func TestSlskdAddRefused(t *testing.T) {
	ctx := context.Background()
	f := &fakeSlskd{failAll: true}
	e := NewSlskd(startSlskd(t, f), slskdKey, nil)
	if _, err := e.Add(ctx, AddRequest{URL: sources.SlskdLink(slskdLabRelease)}); err == nil || !strings.Contains(err.Error(), "offline") || errors.Is(err, ErrUncertain) {
		t.Errorf("refused by slskd: %v", err)
	}
	var refused releaseRefused
	if _, err := e.Add(ctx, AddRequest{URL: "slskd:not-base64!"}); !errors.As(err, &refused) {
		t.Errorf("damaged link: %v", err)
	}
	two := sources.SlskdRelease{User: "u", Files: []sources.SlskdFile{{Name: `a\x.flac`, Size: 1}, {Name: `b\y.flac`, Size: 1}}}
	if _, err := e.Add(ctx, AddRequest{URL: sources.SlskdLink(two)}); !errors.As(err, &refused) {
		t.Errorf("two folders: %v", err)
	}
}
