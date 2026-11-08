package main

import (
	"context"
	"errors"
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

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
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
	r = publicReason(fmt.Errorf("%w: /keys/k: %w", secrets.ErrKeyUnreadable, secrets.ErrKeyIsFolder))
	if !strings.Contains(r, "folder") || strings.Contains(r, "does not hold a key") || strings.Contains(r, "chmod") {
		t.Fatalf("key folder reason %q", r)
	}
	// The pointer must say where the guide is.
	if r := publicReason(secrets.ErrWrongKey); !strings.Contains(r, "docs/run/backups.md") {
		t.Fatalf("wrong key reason %q does not say where the guide is", r)
	}
}

// Only key file problems hold the page. Other errors (the database check,
// making a new key) exit as before, so the restart policy tries again, and
// the page never sends a person to restore --pre-update for them.
func TestOnlyKeyFileProblemsHold(t *testing.T) {
	for _, err := range []error{secrets.ErrWrongKey, fmt.Errorf("%w: x", secrets.ErrKeyMissing), fmt.Errorf("%w: x", secrets.ErrKeyUnreadable)} {
		if !keyNeedsPerson(err) {
			t.Fatalf("%v should hold the page", err)
		}
	}
	for _, err := range []error{errors.New("master key: checking the database for saved secrets: database is locked"), fs.ErrExist, nil} {
		if keyNeedsPerson(err) {
			t.Fatalf("%v should not hold the page", err)
		}
	}
}

// A named key file on a share that mounts after the app starts: the page
// shows it is not there, and once it appears the app exits so the restart
// policy starts it with the key.
func TestNamedKeyThatAppearsLaterRestarts(t *testing.T) {
	old := keyRecheck
	keyRecheck = 50 * time.Millisecond
	t.Cleanup(func() { keyRecheck = old })
	dir := t.TempDir()
	named := filepath.Join(t.TempDir(), "late.key")
	t.Setenv(secrets.EnvKeyFile, named)
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
	held := false
	for i := 0; i < 100 && !held; i++ {
		select {
		case err := <-done:
			t.Fatalf("serve exited before the key file appeared: %v", err)
		default:
		}
		if resp, err := http.Get("http://" + addr + "/healthz"); err == nil {
			held = resp.StatusCode == 503
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !held {
		t.Fatal("no holding page for the missing named key")
	}
	if err := os.WriteFile(named, []byte(strings.Repeat("ab", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("serve ended without an error, so a restart policy would not start it again")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve kept the page after the key file appeared")
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
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "run", "backups.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "## Move to a new machine")
	if i < 0 {
		t.Fatal(`docs/run/backups.md has no "Move to a new machine" section`)
	}
	sec := s[i:]
	if j := strings.Index(sec[3:], "\n## "); j >= 0 {
		sec = sec[:j+3]
	}
	// Named volumes and NAS boxes have no host folder or shell: the key and
	// the zip go in with docker cp, into a container made but not started,
	// and the one-off restore carries a named key file's setting.
	for _, want := range []string{"Before the first start", "holyarrs restore", secrets.KeyFile, "docker cp", "create", "-e " + secrets.EnvKeyFile} {
		if !strings.Contains(sec, want) {
			t.Errorf("section does not say %q", want)
		}
	}
}
