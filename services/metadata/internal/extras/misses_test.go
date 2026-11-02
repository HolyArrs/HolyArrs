package extras

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Cache audit finding 13: an upstream error is re-asked after 15 minutes;
// a real "none" is kept a day.
func TestErrorMissShorterThanNone(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/movies/1" {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(404)
	}))
	defer up.Close()
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	f := &Fanart{Key: fakeKey, URL: up.URL, Now: func() time.Time { return now }}
	_, _, _ = f.Art(context.Background(), "movie", "1")
	_, _, _ = f.Art(context.Background(), "movie", "2")
	if calls.Load() != 2 {
		t.Fatalf("calls %d", calls.Load())
	}
	now = now.Add(16 * time.Minute)
	_, _, _ = f.Art(context.Background(), "movie", "1")
	_, _, _ = f.Art(context.Background(), "movie", "2")
	if calls.Load() != 3 {
		t.Fatalf("after 16 min: error re-asked, none kept; calls %d", calls.Load())
	}
}
