package main

import (
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
)

// The container's health check always calls the root /healthz on the
// default port, whatever URL base is set (HOLYARRS_URL_BASE, the flag or
// Settings, which the probe cannot see). HOLYARRS_HEALTH_URL still wins.
func TestHealthcheckCmdCallsRootUnderURLBase(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+defaultPort)
	if err != nil {
		t.Skipf("port %s is in use here: %v", defaultPort, err)
	}
	var mu sync.Mutex
	var paths []string
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path != "/healthz" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	t.Setenv("HOLYARRS_HEALTH_URL", "")
	t.Setenv("FILE__HOLYARRS_HEALTH_URL", "")
	_ = os.Unsetenv("FILE__HOLYARRS_HEALTH_URL")
	t.Setenv("FILE__HOLYARRS_URL_BASE", "")
	_ = os.Unsetenv("FILE__HOLYARRS_URL_BASE")
	t.Setenv("HOLYARRS_URL_BASE", "/holyarrs")
	got := healthcheckCmd()
	mu.Lock()
	defer mu.Unlock()
	if got != 0 {
		t.Fatalf("health check under a URL base = %d, want 0 (paths asked: %v)", got, paths)
	}
	if len(paths) != 1 || paths[0] != "/healthz" {
		t.Fatalf("health check asked %v, want only the root /healthz", paths)
	}
}
