package subtitles

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func routeCode(m *http.ServeMux, method, path string) int {
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader("{}")))
	return rec.Code
}

// Subtitle sites and searching are on with no switch set (1.1.1).
func TestRoutes_SitesOnWithNoSwitch(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	f := newFix(t)
	m := http.NewServeMux()
	NewHandlers(f.st, nil, &Service{Store: f.st}).Routes(m)
	for _, p := range []string{"/api/v1/subtitles/languages", "/api/v1/settings/subtitles", "/api/v1/subtitle-sites", "/api/v1/subtitle-sites/kinds"} {
		if c := routeCode(m, "GET", p); c != http.StatusOK {
			t.Errorf("GET %s: %d, want 200", p, c)
		}
	}
	if c := routeCode(m, "POST", "/api/v1/subtitles/search"); c == http.StatusNotFound || c == http.StatusMethodNotAllowed {
		t.Errorf("search route missing: %d", c)
	}
}
