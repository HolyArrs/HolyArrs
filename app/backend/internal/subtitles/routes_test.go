package subtitles

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

func routeCode(m *http.ServeMux, method, path string) int {
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader("{}")))
	return rec.Code
}

func TestRoutes_SitesOffKeepsBaseRoutes(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	f := newFix(t)
	m := http.NewServeMux()
	NewHandlers(f.st, nil, &Service{Store: f.st}).Routes(m)
	for _, p := range []string{"/api/v1/subtitles/languages", "/api/v1/settings/subtitles"} {
		if c := routeCode(m, "GET", p); c != http.StatusOK {
			t.Errorf("GET %s with sites off: %d, want 200", p, c)
		}
	}
	if c := routeCode(m, "POST", "/api/v1/subtitles/search"); c != http.StatusNotFound {
		t.Errorf("search with sites off: %d, want 404", c)
	}
	if c := routeCode(m, "GET", "/api/v1/subtitle-sites"); c != http.StatusNotFound {
		t.Errorf("site list with sites off: %d, want 404", c)
	}

	// With the feature flag on, the site routes are registered as well.
	t.Setenv("HOLYARRS_FLAGS", startup.SubtitleSites)
	m = http.NewServeMux()
	NewHandlers(f.st, nil, &Service{Store: f.st}).Routes(m)
	if c := routeCode(m, "GET", "/api/v1/subtitles/languages"); c != http.StatusOK {
		t.Errorf("languages with sites on: %d", c)
	}
	if c := routeCode(m, "GET", "/api/v1/subtitle-sites/kinds"); c == http.StatusNotFound {
		t.Errorf("site kinds with sites on: %d, want a registered route", c)
	}
}
