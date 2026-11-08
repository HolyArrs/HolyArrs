package library

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 5.2 review: folder names are compared case-insensitively only where the
// filesystem folds case (a probe of the Library folder).
func TestFoldsCaseProbe(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Probe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, upErr := os.Stat(filepath.Join(filepath.Dir(dir), "PROBE"))
	want := upErr == nil
	if got := FoldsCase(dir); got != want {
		t.Fatalf("foldsCase = %v, the filesystem says %v", got, want)
	}
	if runtime.GOOS == "windows" && !FoldsCase(dir) {
		t.Fatal("windows folds case")
	}
	if sameFolderName("Lab Show", "lab show", true) != true || sameFolderName("Lab Show", "lab show", false) != false || !sameFolderName("A", "A", false) {
		t.Fatal("sameFolderName")
	}
	// A folder with no letters is probed through a parent that has some.
	num := filepath.Join(dir, "001")
	_ = os.MkdirAll(num, 0o755)
	if FoldsCase(num) != want {
		t.Fatal("probe through a parent")
	}
}
