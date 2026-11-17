package metadata

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// A server that only asks HolyArrs to slow down is not reported as down:
// once it answers again, the next call goes through.
func TestBusyAnswersDoNotMarkTheServerDown(t *testing.T) {
	outbound.Reset()
	t.Cleanup(outbound.Reset)
	var busy atomic.Bool
	busy.Store(true)
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if busy.Load() {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		serviceReply(w, "tmdb", "1", map[string]any{"id": 1})
	})
	for range 4 {
		if _, err := s.detail(context.Background(), "tmdb", "movie", "1"); !errors.Is(err, ErrServiceBusy) {
			t.Fatalf("busy answer: %v", err)
		}
	}
	busy.Store(false)
	// The call reaches the server again (its answer's shape does not matter here).
	if _, err := s.detail(context.Background(), "tmdb", "movie", "1"); errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("after busy answers the server must not be marked down: %v", err)
	}
}

// A health check answered with a busy 503 or 429 that names a wait is the
// server pacing HolyArrs, not an outage.
func TestHealthBusyWithRetryAfterIsUp(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		s := testService(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"service_busy"}}`))
		})
		if !(&Health{Service: s}).Up(context.Background()) {
			t.Fatalf("%d busy with Retry-After reported down", status)
		}
	}
	plain := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	if (&Health{Service: plain}).Up(context.Background()) {
		t.Fatal("plain 503 must still be down")
	}
}
