package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The program serves the routes of the modules: a
// registration missing from routes would turn these into the web app's page.
func TestRoutesServeModuleRoutes(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := routes(buildHandlers(services{Store: s, Version: "test"}), config{})
	for _, path := range []string{"/api/v1/sources", "/api/v1/bridges", "/api/v1/searches", "/api/v1/settings/search", "/api/v1/held"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Errorf("GET %s: %d %s", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}
