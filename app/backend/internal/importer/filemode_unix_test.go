//go:build unix

package importer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

var umaskCases = []struct {
	umask     int
	file, dir fs.FileMode
}{{0o022, 0o644, 0o755}, {0o002, 0o664, 0o775}}

func withUmask(t *testing.T, m int) {
	old := syscall.Umask(m)
	t.Cleanup(func() { syscall.Umask(old) })
}

func permOf(t *testing.T, p string) fs.FileMode {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestMediaModesFollowTheUmask(t *testing.T) {
	for _, c := range umaskCases {
		t.Run(fmt.Sprintf("umask %03o", c.umask), func(t *testing.T) {
			withUmask(t, c.umask)
			root := t.TempDir()
			src := filepath.Join(root, "src.mkv")
			if err := os.WriteFile(src, []byte("movie"), 0o600); err != nil {
				t.Fatal(err)
			}

			copied := filepath.Join(root, "copy.mkv")
			if err := CopyNoReplace(src, copied); err != nil {
				t.Fatal(err)
			}
			if got := permOf(t, copied); got != c.file {
				t.Fatalf("copy is %04o, want %04o", got, c.file)
			}

			linkHook = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} }
			t.Cleanup(func() { linkHook = nil })
			direct := filepath.Join(root, "direct.mkv")
			if err := CopyNoReplace(src, direct); err != nil {
				t.Fatal(err)
			}
			linkHook = nil
			if got := permOf(t, direct); got != c.file {
				t.Fatalf("direct copy is %04o, want %04o", got, c.file)
			}

			f, err := CreateTemp(root, ".holyarrs-", ".part")
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			if base := filepath.Base(f.Name()); !strings.HasPrefix(base, ".holyarrs-") || !strings.HasSuffix(base, ".part") {
				t.Fatalf("temp name %q", base)
			}
			if got := permOf(t, f.Name()); got != c.file {
				t.Fatalf("temp file is %04o, want %04o", got, c.file)
			}

			to := filepath.Join(root, "Holding", "2026-11-04", "src.mkv")
			if err := moveFile(src, to); err != nil {
				t.Fatal(err)
			}
			for _, d := range []string{filepath.Join(root, "Holding"), filepath.Dir(to)} {
				if got := permOf(t, d); got != c.dir {
					t.Fatalf("%s is %04o, want %04o", d, got, c.dir)
				}
			}
		})
	}
}
