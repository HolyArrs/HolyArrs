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
