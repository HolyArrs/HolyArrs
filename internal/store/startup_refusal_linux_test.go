//go:build linux

package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Statfs magic numbers: the refusals decide on these alone.
func TestFilesystemKinds(t *testing.T) {
	cases := map[int64]string{
		0x01021997: "9p",
		0x65735546: "fuse",
		0x6969:     "nfs",
		0xff534d42: "cifs",
		0x6a656a63: "fakeowner", // Docker Desktop for Mac 4.74-4.96 binds
		0x01021994: "tmpfs",
		0x858458f6: "ramfs",
		0xef53:     "", // ext2/3/4
		0x58465342: "", // xfs
		0x9123683e: "", // btrfs
		0x2fc12fc1: "", // zfs
		0x794c7630: "", // overlayfs
	}
	for magic, want := range cases {
		if got := fsKindOf(magic); got != want {
			t.Errorf("fsKindOf(%#x) = %q, want %q", magic, got, want)
		}
	}
}

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
	if strings.Contains(err.Error(), "readonly database") {
		t.Fatalf("SQLite's words leaked: %v", err)
	}
}
