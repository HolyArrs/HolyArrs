//go:build linux

package database

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A /config that cannot be written gets a plain, classified error before
// SQLite says "readonly database".
func TestOpenNamesAReadOnlyConfigFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anyway")
	}
	dir := filepath.Join(t.TempDir(), "config")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := Open(dir, Options{NetworkShare: noShare})
	if !errors.Is(err, ErrNotWritable) {
		t.Fatalf("got %v, want ErrNotWritable", err)
	}
	if !errors.Is(err, syscall.EACCES) {
		t.Errorf("the cause is lost: %v", err)
	}
	if strings.Contains(err.Error(), "readonly database") {
		t.Fatalf("SQLite's words leaked: %v", err)
	}
}
