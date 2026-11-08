package database

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The databases hold password and key hashes, so they are owner-only even
// when the umask would leave them readable by others.
func TestDatabaseFilesOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not Unix permissions on Windows")
	}
	s, dir := openTest(t, Options{})
	snap := t.TempDir()
	mainCopy, evCopy, err := s.Snapshot(snap)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(dir, MainFile), filepath.Join(dir, EventsFile), mainCopy, evCopy} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s is %v", p, fi.Mode().Perm())
		}
	}
}
