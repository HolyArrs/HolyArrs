package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// A missing master key for a database with saved secrets: the app keeps
// listening and says what to do, instead of exiting into a restart loop
// that NAS users only see in the container log.
func TestMissingKeyKeepsAPlainPage(t *testing.T) {
	t.Setenv(secrets.EnvKeyFile, "")
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Put(context.Background(), "source_api_key", "lab-value"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err := os.Remove(filepath.Join(dir, secrets.KeyFile)); err != nil {
		t.Fatal(err)
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()
	c, err := parseFlags([]string{"-config", dir, "-addr", addr}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, runOpts{}) }()
	var resp *http.Response
	for i := 0; i < 100; i++ {
		select {
		case err := <-done:
			cancel()
			t.Fatalf("serve exited instead of keeping a page: %v", err)
		default:
		}
		if resp, err = http.Get("http://" + addr + "/healthz"); err == nil && resp.StatusCode == 503 {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || resp.StatusCode != 503 {
		cancel()
		t.Fatalf("healthz: %v %v", resp, err)
	}
	resp.Body.Close()
	resp, err = http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "master key file is missing") || strings.Contains(string(b), dir) {
		t.Fatalf("page %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, secrets.KeyFile)); err == nil {
		t.Fatal("a new key was made")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("serve ended without the error")
	}
}
