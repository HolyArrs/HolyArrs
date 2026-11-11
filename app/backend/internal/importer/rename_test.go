package importer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRenameNoReplace(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv")
	write(t, src, 10)
	if err := renameNoReplace(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the source is still there")
	}
	if fi, err := os.Stat(dst); err != nil || fi.Size() != 10 {
		t.Fatalf("target: %v", err)
	}
	other := filepath.Join(dir, "c.mkv")
	write(t, other, 3)
	if err := renameNoReplace(other, dst); !errors.Is(err, fs.ErrExist) {
		t.Errorf("onto an existing file: %v", err)
	}
	if fi, _ := os.Stat(dst); fi.Size() != 10 {
		t.Error("an existing file was replaced")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("the refused source went missing")
	}
}

// Without no-replace support the check-then-rename path still never replaces a file.
func TestRenameNoReplaceFallback(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv")
	write(t, src, 10)
	write(t, dst, 3)
	if err := renameChecked(src, dst); !errors.Is(err, fs.ErrExist) {
		t.Errorf("onto an existing file: %v", err)
	}
	if fi, _ := os.Stat(dst); fi.Size() != 3 {
		t.Error("an existing file was replaced")
	}
	_ = os.Remove(dst)
	if err := renameChecked(src, dst); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dst); err != nil || fi.Size() != 10 {
		t.Fatalf("target: %v", err)
	}
}

// When the finished part cannot be hardlinked, it is renamed into place: the
// source is read once, not twice.
func TestCopyWritesOnceWhenLinkUnsupported(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.mkv"), filepath.Join(dir, "out", "dst.mkv")
	write(t, src, 100)
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	linkHook = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EPERM} }
	t.Cleanup(func() { linkHook = nil })
	opens := 0
	openHook = func(p string) {
		if p == src {
			opens++
		}
	}
	t.Cleanup(func() { openHook = nil })
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if opens != 1 {
		t.Errorf("the source was read %d times", opens)
	}
	if fi, _ := os.Stat(dst); fi == nil || fi.Size() != 100 {
		t.Fatal("copy wrong")
	}
	if e, _ := os.ReadDir(filepath.Dir(dst)); len(e) != 1 || strings.HasSuffix(e[0].Name(), ".part") {
		t.Fatalf("temp file left behind: %v", e)
	}
}

// Any link error other than "exists" still ends in one finished copy.
func TestCopyRenamesOnOtherLinkErrors(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.mkv"), filepath.Join(dir, "dst.mkv")
	write(t, src, 50)
	linkHook = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.Errno(38)} } // ENOSYS on Linux
	t.Cleanup(func() { linkHook = nil })
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(dst); fi == nil || fi.Size() != 50 {
		t.Fatal("copy wrong")
	}
}
