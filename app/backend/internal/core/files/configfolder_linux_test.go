//go:build linux

package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMountTypeRoot(t *testing.T) {
	if got := MountType("/"); got == "" {
		t.Fatal("no type for /")
	}
	d := t.TempDir()
	link := filepath.Join(t.TempDir(), "l")
	if err := os.Symlink(d, link); err != nil {
		t.Skip("no symlinks:", err)
	}
	if a, b := MountType(link), MountType(d); a != b {
		t.Fatalf("symlink %q vs target %q", a, b)
	}
}
