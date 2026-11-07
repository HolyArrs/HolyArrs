package metadata

import (
	"context"
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestOutsideBreakerUsesStableMetadataEndpointIdentity(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	m := &TVMaze{BaseURL: srv.URL, HTTP: srv.Client()}
	for _, query := range []string{"one", "two", "three"} {
		if _, err := m.SearchShows(context.Background(), query); err == nil {
			t.Fatal("failing metadata call returned no error")
		}
	}
	if _, err := m.SearchShows(context.Background(), "four"); !errors.Is(err, outbound.ErrSuppressed) {
		t.Fatalf("fourth query was not suppressed: %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("server hits = %d, want 3", got)
	}
}
