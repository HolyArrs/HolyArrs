package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// The picture cache is 5 GB unless Settings chose another size; a choice
// outside 1, 2, 5, 10 and 20 GB is refused.
func TestArtworkCacheSizeSetting(t *testing.T) {
	t.Parallel()
	a, _ := signinRig(t, nil)
	if gb := health.UISettingsOf(context.Background(), a.s.Store).ArtworkCacheGB; gb != 5 {
		t.Fatalf("default %d GB, want 5", gb)
	}
	put := func(body string) int {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("PUT", "/api/v1/settings/ui", strings.NewReader(body)))
		return w.Code
	}
	if c := put(`{"artwork_cache_gb": 7}`); c != 400 {
		t.Fatalf("7 GB answered %d", c)
	}
	if c := put(`{"artwork_cache_gb": 10}`); c != 200 || health.UISettingsOf(context.Background(), a.s.Store).ArtworkCacheGB != 10 {
		t.Fatalf("10 GB not kept (%d)", c)
	}
}
