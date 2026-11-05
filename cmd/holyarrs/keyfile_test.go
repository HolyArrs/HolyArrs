package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/secrets"
)

// The key messages point at the docs for moving to a new machine, and an
// unreadable key file says how to fix the owner on the host.
func TestKeyReasonsSayWhatToDo(t *testing.T) {
	for _, err := range []error{secrets.ErrWrongKey, fmt.Errorf("%w: x", secrets.ErrKeyMissing)} {
		if r := publicReason(err); !strings.Contains(r, "Move to a new machine") {
			t.Fatalf("%v: reason %q", err, r)
		}
	}
	r := publicReason(fmt.Errorf("%w: open /keys/k: %w", secrets.ErrKeyUnreadable, fs.ErrPermission))
	for _, want := range []string{"cannot be read", "chown PUID:PGID", "chmod 600"} {
		if !strings.Contains(r, want) {
			t.Fatalf("unreadable key reason %q lacks %q", r, want)
		}
	}
	r = publicReason(fmt.Errorf("%w: open /keys/k: %w", secrets.ErrKeyUnreadable, fs.ErrNotExist))
	if !strings.Contains(r, secrets.EnvKeyFile) {
		t.Fatalf("missing named key reason %q", r)
	}
}

// A key file that cannot be used holds the plain page instead of exiting
// into a restart loop.
func TestDamagedKeyKeepsAPlainPage(t *testing.T) {
	t.Setenv(secrets.EnvKeyFile, "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, secrets.KeyFile), []byte("not a key\n"), 0o600); err != nil {
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
	if !strings.Contains(string(b), "cannot be read") {
		t.Fatalf("page %q", b)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("serve ended without the error")
	}
}

// The key messages point at this section, so it must be there and name the
// way to restore without the app running.
func TestMoveToNewMachineDocs(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "backups.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "## Move to a new machine")
	if i < 0 {
		t.Fatal(`docs/backups.md has no "Move to a new machine" section`)
	}
	sec := s[i:]
	if j := strings.Index(sec[3:], "\n## "); j >= 0 {
		sec = sec[:j+3]
	}
	for _, want := range []string{"Before the first start", "holyarrs restore", secrets.KeyFile} {
		if !strings.Contains(sec, want) {
			t.Errorf("section does not say %q", want)
		}
	}
}
