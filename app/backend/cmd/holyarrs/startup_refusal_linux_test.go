//go:build linux

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startRefused runs serve on dir and returns the healthz code, the page and
// how serve ended after cancel. A refusal a person must fix keeps answering
// (the container stays up, unhealthy) instead of exiting into a restart loop.
func startRefused(t *testing.T, dir string) (int, string) {
	t.Helper()
	// If the refusal is missing the app starts for real: keep it off the
	// real metadata server.
	t.Setenv("HOLYARRS_METADATA_URL", "https://meta.invalid")
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()
	c, err := parseFlags([]string{"-config", dir, "-addr", addr}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, runOpts{}) }()
	code := 0
	for i := 0; i < 100; i++ {
		select {
		case err := <-done:
			t.Fatalf("serve exited instead of showing a page (restart loop): %v", err)
		default:
		}
		if resp, err := http.Get("http://" + addr + "/healthz"); err == nil {
			code = resp.StatusCode
			resp.Body.Close()
			if code == http.StatusServiceUnavailable {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop")
	}
	return code, string(b)
}

func isTmpfs(dir string) bool {
	var st syscall.Statfs_t
	return syscall.Statfs(dir, &st) == nil && int64(st.Type)&0xffffffff == 0x01021994
}

// A /config that cannot be written (:ro, or not owned by PUID/PGID) shows
// the page instead of exiting into a restart loop: the log folder and the
// lock file come before the store's own check.
func TestReadOnlyConfigShowsAPage(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anyway")
	}
	dir := filepath.Join(t.TempDir(), "config")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	code, page := startRefused(t, dir)
	if code != http.StatusServiceUnavailable || !strings.Contains(page, "cannot be written") || strings.Contains(page, dir) {
		t.Fatalf("healthz %d, page %q", code, page)
	}
}

// Unraid with a pool path that does not exist: /config lands in RAM.
func TestConfigInMemoryShowsAPage(t *testing.T) {
	if !isTmpfs("/dev/shm") {
		t.Skip("/dev/shm is not tmpfs here")
	}
	dir, err := os.MkdirTemp("/dev/shm", "holyarrs-mem-*")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	code, page := startRefused(t, dir)
	if code != http.StatusServiceUnavailable || !strings.Contains(page, "memory") || strings.Contains(page, dir) {
		if len(page) > 300 {
			page = page[:300] + "..."
		}
		t.Fatalf("healthz %d, page %q", code, page)
	}
	if _, err := os.Stat(filepath.Join(dir, "holyarrs.db")); err == nil {
		t.Fatal("database created in memory")
	}
}
