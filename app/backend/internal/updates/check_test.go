package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeFeed struct {
	srv   *httptest.Server
	calls atomic.Int32
	code  int
	body  string
}

func newFeed(t *testing.T) *fakeFeed {
	f := &fakeFeed{code: 200}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.calls.Add(1)
		w.WriteHeader(f.code)
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestUpdateCheckDailyFromTheFeed(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	// No setting and no default feed: nothing is asked.
	if _, ok := w.run(t)["update"]; ok || f.calls.Load() != 0 {
		t.Fatal("asked without a feed")
	}
	w.e.ReleaseFeed = f.srv.URL
	f.body = `{"tag_name": "v1.2.0", "html_url": "https://example.com/r/v1.2.0"}`
	if c := w.run(t)["update"]; c.State != Warning || c.Message != "HolyArrs v1.2.0 is out (this is v1.1.0)" {
		t.Fatalf("newer: %+v", c)
	}
	info := w.e.Update(context.Background())
	if !info.On || !info.Newer || info.Link != "" || info.CheckedAt == nil {
		t.Fatalf("info %+v", info)
	}
	w.run(t)
	if f.calls.Load() != 1 {
		t.Fatalf("asked %d times within a day", f.calls.Load())
	}
	// A restart reads the last answer and does not ask again.
	e2 := &Checker{Store: w.s, Version: "v1.1.0", ReleaseFeed: f.srv.URL, Now: func() time.Time { return w.now }}
	if i := e2.Update(context.Background()); !i.Newer {
		t.Fatalf("not kept: %+v", i)
	}
	w.now = w.now.Add(UpdateEvery)
	f.code, f.body = 404, `{"message":"Not Found"}`
	if c := w.run(t)["update"]; c.State != OK || f.calls.Load() != 2 {
		t.Fatalf("404 not quiet: %+v", c)
	}
	if i := w.e.Update(context.Background()); !i.NoRelease || i.Newer || i.Problem != "" {
		t.Fatalf("404 info %+v", i)
	}
	f.code, f.body = 200, `[]`
	if i := w.e.CheckUpdate(context.Background()); !i.NoRelease {
		t.Fatalf("empty list %+v", i)
	}
	f.body = strings.Repeat("x", updateMaxBody+10)
	if i := w.e.CheckUpdate(context.Background()); !strings.Contains(i.Problem, "too large") {
		t.Fatalf("big %+v", i)
	}
	f.code = 500
	if c := w.run(t)["update"]; c.State != OK {
		t.Fatalf("a failed check is a warning: %+v", c)
	}
}

func TestUpdateCheckOffAsksNothing(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	f.body = "v9.0.0"
	w.e.ReleaseFeed = f.srv.URL
	w.exec(t, "INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', 'update_check', 'off', 't', '{}', 't', '{}')")
	if _, ok := w.run(t)["update"]; ok {
		t.Fatal("checked while off")
	}
	if i := w.e.CheckUpdate(context.Background()); i.On || f.calls.Load() != 0 {
		t.Fatalf("off asked: %d %+v", f.calls.Load(), i)
	}
}

func TestUpdateFeedSettingWinsAndPlainText(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	f.body = "v1.0.0\n"
	w.e.ReleaseFeed = "http://127.0.0.1:1/never"
	w.exec(t, "INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', 'update_check_url', ?, 't', '{}', 't', '{}')", f.srv.URL)
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.0.0" || i.Newer || i.Feed != f.srv.URL {
		t.Fatalf("%+v", i)
	}
}

func TestUpdateCheckEveryDays(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	w.e.ReleaseFeed = f.srv.URL
	w.exec(t, "INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', 'update_check_every_days', '7', 't', '{}', 't', '{}')")
	w.run(t)
	w.now = w.now.Add(6 * UpdateEvery)
	w.run(t)
	if f.calls.Load() != 1 {
		t.Fatalf("asked %d times within a week", f.calls.Load())
	}
	w.now = w.now.Add(UpdateEvery)
	w.run(t)
	if f.calls.Load() != 2 {
		t.Fatalf("asked %d times after a week", f.calls.Load())
	}
}
