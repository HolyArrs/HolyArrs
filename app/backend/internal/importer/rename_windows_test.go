//go:build windows

package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLongPathPrefix(t *testing.T) {
	long := "C:\\" + strings.Repeat("a", 300)
	if got := longPath(long); got != "\\\\?\\"+long {
		t.Errorf("drive path: %q", got[:8])
	}
	share := "\\\\nas\\media\\" + strings.Repeat("b", 300)
	if got := longPath(share); got != "\\\\?\\UNC\\nas\\media\\"+strings.Repeat("b", 300) {
		t.Errorf("share path: %q", got[:16])
	}
	if got := longPath("C:\\short.mkv"); got != "C:\\short.mkv" {
		t.Errorf("short path: %q", got)
	}
}

// A move into a folder deeper than 260 characters works and writes nothing twice.
func TestRenameNoReplaceLongPath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	write(t, src, 10)
	deep := dir
	for len(deep) < 280 {
		deep = filepath.Join(deep, strings.Repeat("d", 40))
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(deep, "Movie (2024) 1080p WEB.mkv")
	if err := renameNoReplace(src, dst); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dst); err != nil || fi.Size() != 10 {
		t.Fatalf("target: %v", err)
	}
}
