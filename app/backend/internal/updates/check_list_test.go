package updates_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates/signtest"
)

// listServer serves a signed update list at /manifest.json.
type listServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	down bool
	hits int
	data []byte
	sig  []byte
}

func newListServer(t *testing.T) *listServer {
	l := &listServer{}
	l.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.hits++
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
		`","changelog_url":"https://github.com/HolyArrs/HolyArrs/releases/tag/` + version + `","notes_short":"<i>Fixes</i>"` + extra + `}},"revoked":[]}`)
	l.sig = k.Sign(l.data, "serial "+strconv.Itoa(serial))
}

func listWorld(t *testing.T) (*world, signtest.Key, *listServer, *listServer) {
	w := newWorld(t)
	k := signtest.New(t)
	srv, pages := newListServer(t), newListServer(t)
	w.e.ListKeys = []updates.PublicKey{k.Pub}
	w.e.ListURL, w.e.ListCopyURL = srv.url(), pages.url()
	return w, k, srv, pages
}

func TestUpdateListGood(t *testing.T) {
	w, k, srv, _ := listWorld(t)
	srv.set(k, 3, "v1.2.0", "")
	i := w.e.CheckUpdate(context.Background())
	if !i.Newer || i.Latest != "v1.2.0" || i.Notes != "Fixes" || i.Link != "https://github.com/HolyArrs/HolyArrs/releases/tag/v1.2.0" {
		t.Fatalf("info %+v", i)
	}
	if w.e.Setting(context.Background(), updates.SettingListSerial) != "3" {
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
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "" || i.Problem != updates.SignatureProblem {
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
	for n := 1; n < updates.ListFailuresForCopy; n++ {
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
	_ = w.e.SaveSetting(context.Background(), updates.SettingListFailures, "5")
	srv.set(k, 5, "v1.2.0", "")
	pages.set(k, 7, "v1.3.0", "")
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.3.0" {
		t.Fatalf("pages newer: %+v", i)
	}
	_ = w.e.SaveSetting(context.Background(), updates.SettingListFailures, "5")
	srv.set(k, 8, "v1.4.0", "")
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.4.0" {
		t.Fatalf("server newer: %+v", i)
	}
}

func TestUpdateListRolloutHoldRevoked(t *testing.T) {
	w, k, srv, _ := listWorld(t)
	ctx := context.Background()
	_ = w.e.SaveSetting(ctx, updates.SettingListBucket, "40")
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
	b := w.e.Bucket(ctx)
	if b < 0 || b > 99 || w.e.Bucket(ctx) != b {
		t.Fatalf("bucket %d not kept", b)
	}
}

func TestUpdateListEmptyPinnedKeysKeepsGitHub(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	srv := newListServer(t)
	w.e.ReleaseFeed, w.e.ListURL = f.srv.URL, srv.url()
	f.body = `{"tag_name": "v1.2.0"}`
	if w.e.ListMode() {
		t.Fatal("list mode without a pinned key")
	}
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.2.0" || f.calls.Load() != 1 {
		t.Fatalf("old path: %+v", i)
	}
}

// Release proof B2: with the check turned off, no list is asked for at
// all, and turning it back on reports the newer release again.
func TestUpdateListCheckOffAsksNothing(t *testing.T) {
	w, k, srv, pages := listWorld(t)
	srv.set(k, 3, "v1.2.0", "")
	ctx := context.Background()
	if err := w.e.SaveSetting(ctx, updates.SettingUpdateOn, "off"); err != nil {
		t.Fatal(err)
	}
	if i := w.e.CheckUpdate(ctx); i.On || i.Newer || i.Latest != "" {
		t.Fatalf("off: %+v", i)
	}
	if srv.hits != 0 || pages.hits != 0 {
		t.Fatalf("asked with the check off: %d + %d", srv.hits, pages.hits)
	}
	if err := w.e.SaveSetting(ctx, updates.SettingUpdateOn, "on"); err != nil {
		t.Fatal(err)
	}
	if i := w.e.CheckUpdate(ctx); !i.Newer || i.Latest != "v1.2.0" || i.Link == "" {
		t.Fatalf("on again: %+v", i)
	}
}
