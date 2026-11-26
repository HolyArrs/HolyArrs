//go:build unix

package accounts

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A pipe named as a value file is refused at once instead of holding the
// start until something writes to it.
func TestEnvSetupRefusesAPipe(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skip(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readValueFile(EnvOwnerPassword, p); done <- err }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("pipe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading a pipe held the start")
	}
}

// A pipe's refusal leaves its path off the page too.
func TestEnvSetupPipeRefusalHidesThePath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skip(err)
	}
	assertNoPath(t, p, EnvOwnerPassword, "pipe")
	assertNoPath(t, p, EnvAPIKey, "pipe")
}
