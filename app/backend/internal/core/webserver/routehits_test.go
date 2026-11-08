package webserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOnlyServedRoutesCount(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/a/{id}", func(http.ResponseWriter, *http.Request) {})
	m.HandleFunc("POST /api/v1/b", func(http.ResponseWriter, *http.Request) {})
	refuse := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusForbidden) // never reaches the mux
				return
			}
			h.ServeHTTP(w, r)
		})
	}
	srv := httptest.NewServer(refuse(RecordRouteHits(m)))
	defer srv.Close()
	for _, c := range []struct{ method, path string }{{"GET", "/api/v1/a/7"}, {"POST", "/api/v1/b"}, {"GET", "/nope"}} {
		req, _ := http.NewRequest(c.method, srv.URL+c.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if u := UnhitRoutes([]string{"GET /api/v1/a/{id}", "POST /api/v1/b"}); len(u) != 1 || u[0] != "POST /api/v1/b" {
		t.Fatalf("unhit %v", u)
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "x.go"), []byte("m.HandleFunc(\"GET /api/v1/a/{id}\", h)\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "x_test.go"), []byte("m.HandleFunc(\"GET /api/v1/test-only\", h)\n"), 0o644)
	if r, err := RegisteredRoutes(dir); err != nil || len(r) != 1 || r[0] != "GET /api/v1/a/{id}" {
		t.Fatalf("registered %v %v", r, err)
	}
}
