package secrets

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Every way the key file cannot be used is marked, so the start can hold a
// plain page instead of exiting into a restart loop.
func TestLoadKeyFileProblemsAreMarked(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadKey(dir, filepath.Join(dir, "gone.key")); !errors.Is(err, ErrKeyUnreadable) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing named file: %v", err)
	}
	bad := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(bad, []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(dir, bad); !errors.Is(err, ErrKeyUnreadable) {
		t.Fatalf("damaged named file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, KeyFile), []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(dir, ""); !errors.Is(err, ErrKeyUnreadable) {
		t.Fatalf("damaged key in the config folder: %v", err)
	}
}

func TestLoadKeyNoPermissionIsMarked(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix modes and a non-root user")
	}
	dir := t.TempDir()
	if _, err := LoadKey(dir, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, KeyFile), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(dir, ""); !errors.Is(err, ErrKeyUnreadable) || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable key: %v", err)
	}
}

// A key path that is a folder (Docker makes one when the mounted host file
// did not exist) is told apart from a damaged key.
func TestLoadKeyFolderIsMarked(t *testing.T) {
	dir := t.TempDir()
	named := filepath.Join(dir, "named.key")
	if err := os.Mkdir(named, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(dir, named); !errors.Is(err, ErrKeyUnreadable) || !errors.Is(err, ErrKeyIsFolder) {
		t.Fatalf("named key folder: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, KeyFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(dir, ""); !errors.Is(err, ErrKeyUnreadable) || !errors.Is(err, ErrKeyIsFolder) {
		t.Fatalf("key folder in the config folder: %v", err)
	}
}
