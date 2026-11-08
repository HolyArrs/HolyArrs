//go:build unix

package library

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMovedMediaFoldersFollowTheUmask(t *testing.T) {
	for _, c := range []struct {
		umask     int
		file, dir fs.FileMode
	}{{0o022, 0o644, 0o755}, {0o002, 0o664, 0o775}} {
		t.Run(fmt.Sprintf("umask %03o", c.umask), func(t *testing.T) {
			old := syscall.Umask(c.umask)
			t.Cleanup(func() { syscall.Umask(old) })
			root := t.TempDir()
			from := filepath.Join(root, "old", "movie.mkv")
			if err := os.MkdirAll(filepath.Dir(from), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(from, []byte("movie"), 0o600); err != nil {
				t.Fatal(err)
			}
			// Another disk: the link fails and the file is copied.
			linkFile = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} }
			t.Cleanup(func() { linkFile = os.Link })
			to := filepath.Join(root, "Movies", "Dune (2021)", "movie.mkv")
			if err := moveFileDistinct(from, to); err != nil {
				t.Fatal(err)
			}
			for p, want := range map[string]fs.FileMode{to: c.file, filepath.Dir(to): c.dir, filepath.Join(root, "Movies"): c.dir} {
				fi, err := os.Stat(p)
				if err != nil {
					t.Fatal(err)
				}
				if got := fi.Mode().Perm(); got != want {
					t.Fatalf("%s is %04o, want %04o", p, got, want)
				}
			}
		})
	}
}
