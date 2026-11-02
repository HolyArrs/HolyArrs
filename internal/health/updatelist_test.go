package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/updatelist"
	"github.com/CoderLuii/holyarrs/internal/updatelist/signtest"
)

// listServer serves a signed update list at /manifest.json.
type listServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	down bool
	data []byte
	sig  []byte
}

func newListServer(t *testing.T) *listServer {
	l := &listServer{}
	l.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		defer l.mu.Unlock()
		switch {
		case l.down:
			w.WriteHeader(http.StatusServiceUnavailable)
		case r.URL.Path == "/manifest.json":
			_, _ = w.Write(l.data)
		case r.URL.Path == "/manifest.json.minisig":
			_, _ = w.Write(l.sig)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(l.srv.Close)
	return l
}

func (l *listServer) url() string { return l.srv.URL + "/manifest.json" }

func (l *listServer) set(k signtest.Key, serial int, version, extra string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data = []byte(`{"schema":1,"serial":` + strconv.Itoa(serial) + `,"generated_at":"2026-10-13T11:00:00Z","channels":{"stable":{"version":"` + version +
		`","changelog_url":"https://github.com/CoderLuii/holyarrs/releases/tag/` + version + `","notes_short":"<i>Fixes</i>"` + extra + `}},"revoked":[]}`)
	l.sig = k.Sign(l.data, "serial "+strconv.Itoa(serial))
}

func listWorld(t *testing.T) (*world, signtest.Key, *listServer, *listServer) {
	w := newWorld(t)
	k := signtest.New(t)
	srv, pages := newListServer(t), newListServer(t)
	w.e.ListKeys = []updatelist.PublicKey{k.Pub}
	w.e.ListURL, w.e.ListCopyURL = srv.url(), pages.url()
	return w, k, srv, pages
}

func TestUpdateListGood(t *testing.T) {
	w, k, srv, _ := listWorld(t)
	srv.set(k, 3, "v1.2.0", "")
	i := w.e.CheckUpdate(context.Background())
	if !i.Newer || i.Latest != "v1.2.0" || i.Notes != "Fixes" || i.Link != "https://github.com/CoderLuii/holyarrs/releases/tag/v1.2.0" {
		t.Fatalf("info %+v", i)
	}
	if w.e.setting(context.Background(), settingListSerial) != "3" {
		t.Fatal("serial not stored")
	}
	// A replayed older list is ignored; the last answer stays.
	srv.set(k, 2, "v1.3.0", "")
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.2.0" || i.Problem == "" {
		t.Fatalf("older list: %+v", i)
	}
}

func TestUpdateListBadSignatureUsesPages(t *testing.T) {
	w, k, srv, pages := listWorld(t)
	other := signtest.New(t)
	srv.set(other, 9, "v9.0.0", "")
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "" || i.Problem != signatureProblem {
		t.Fatalf("bad signature: %+v", i)
	}
	pages.set(k, 4, "v1.2.0", "")
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.2.0" {
		t.Fatalf("pages copy: %+v", i)
	}
}

func TestUpdateListServerDownThenPages(t *testing.T) {
	w, k, srv, pages := listWorld(t)
	srv.down = true
	pages.set(k, 4, "v1.2.0", "")
	for n := 1; n < listFailuresForCopy; n++ {
		if i := w.e.CheckUpdate(context.Background()); i.Latest != "" {
			t.Fatalf("pages read after %d failures", n)
		}
	}
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.2.0" {
		t.Fatalf("pages not used: %+v", i)
	}
}

func TestUpdateListHigherSerialWins(t *testing.T) {
	w, k, srv, pages := listWorld(t)
	_ = w.e.saveSetting(context.Background(), settingListFailures, "5")
	srv.set(k, 5, "v1.2.0", "")
	pages.set(k, 7, "v1.3.0", "")
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.3.0" {
		t.Fatalf("pages newer: %+v", i)
	}
	_ = w.e.saveSetting(context.Background(), settingListFailures, "5")
	srv.set(k, 8, "v1.4.0", "")
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.4.0" {
		t.Fatalf("server newer: %+v", i)
	}
}

func TestUpdateListRolloutHoldRevoked(t *testing.T) {
	w, k, srv, _ := listWorld(t)
	ctx := context.Background()
	_ = w.e.saveSetting(ctx, settingListBucket, "40")
	srv.set(k, 1, "v1.2.0", `,"rollout_percent":40`)
	if i := w.e.CheckUpdate(ctx); i.Newer {
		t.Fatal("bucket 40 outside rollout 40")
	}
	srv.set(k, 2, "v1.2.0", `,"rollout_percent":41`)
	if i := w.e.CheckUpdate(ctx); !i.Newer {
		t.Fatal("bucket 40 inside rollout 41")
	}
	srv.set(k, 3, "v1.2.0", `,"hold":true`)
	if i := w.e.CheckUpdate(ctx); i.Newer {
		t.Fatal("held release shown")
	}
	srv.mu.Lock()
	srv.data = []byte(`{"schema":1,"serial":4,"generated_at":"2026-10-13T11:00:00Z","channels":{"stable":{"version":"v1.2.0"}},"revoked":["v1.2.0"]}`)
	srv.sig = k.Sign(srv.data, "t")
	srv.mu.Unlock()
	if i := w.e.CheckUpdate(ctx); i.Newer {
		t.Fatal("revoked release shown")
	}
}

func TestUpdateListBucketStoredOnce(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	b := w.e.bucket(ctx)
	if b < 0 || b > 99 || w.e.bucket(ctx) != b {
		t.Fatalf("bucket %d not kept", b)
	}
}

func TestUpdateListEmptyPinnedKeysKeepsGitHub(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	srv := newListServer(t)
	w.e.ReleaseFeed, w.e.ListURL = f.srv.URL, srv.url()
	f.body = `{"tag_name": "v1.2.0"}`
	if w.e.listMode() {
		t.Fatal("list mode without a pinned key")
	}
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.2.0" || f.calls.Load() != 1 {
		t.Fatalf("old path: %+v", i)
	}
}
