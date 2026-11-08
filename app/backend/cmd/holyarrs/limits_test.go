package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
)

func TestLargeBodyGets413(t *testing.T) {
	h := limitBodies(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatal("handler should not see a too-large body")
		}
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/titles", strings.NewReader(strings.Repeat("x", 2<<20))))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestServerTimeouts(t *testing.T) {
	s := newServer(":0", http.NotFoundHandler())
	if s.ReadTimeout != 30*time.Second || s.IdleTimeout != 120*time.Second || s.WriteTimeout != 0 || s.ReadHeaderTimeout == 0 {
		t.Fatalf("%+v", s)
	}
}

func TestHealthcheck(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	if healthcheck(ok.URL) != 0 || healthcheck(bad.URL) != 1 || healthcheck("http://127.0.0.1:1") != 1 {
		t.Fatal("healthcheck wrong")
	}
}

// The health check reads FILE__ variables, so a URL given by file is used.
func TestHealthcheckCmdFileEnv(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/base/healthz" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ok.Close()
	f := filepath.Join(t.TempDir(), "health_url")
	if err := os.WriteFile(f, []byte(ok.URL+"/base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOLYARRS_HEALTH_URL", "http://127.0.0.1:1")
	t.Setenv("FILE__HOLYARRS_HEALTH_URL", f)
	if got := healthcheckCmd(); got != 0 {
		t.Fatalf("health check with FILE__HOLYARRS_HEALTH_URL = %d, want 0", got)
	}
	t.Setenv("FILE__HOLYARRS_HEALTH_URL", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("HOLYARRS_HEALTH_URL", ok.URL+"/base")
	if got := healthcheckCmd(); got != 1 {
		t.Fatalf("health check with an unreadable FILE__ = %d, want 1", got)
	}
}

func TestRestoreUploadSkipsSmallBodyLimit(t *testing.T) {
	var got int64
	h := limitBodies(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.Copy(io.Discard, r.Body)
	}))
	big := strings.Repeat("x", maxBody+10)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", backups.RestorePath, strings.NewReader(big)))
	if got != int64(len(big)) {
		t.Fatalf("restore body cut to %d", got)
	}
}
