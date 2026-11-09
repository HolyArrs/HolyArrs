package updates_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// world is a store and an update Checker for the update list tests.
type world struct {
	s   *database.Store
	e   *updates.Checker
	now time.Time
}

func newWorld(t *testing.T) *world {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w := &world{s: s, now: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	w.e = &updates.Checker{Store: s, Version: "v1.1.0", Now: func() time.Time { return w.now }}
	return w
}

// fakeFeed is a release feed that answers code and body and counts calls.
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
