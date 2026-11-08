//go:build unix

package unrar

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestExtract_ModesFollowUmask checks unpacked files follow the umask.
func TestExtract_ModesFollowUmask(t *testing.T) {
	for _, c := range []struct {
		umask     int
		file, dir fs.FileMode
	}{{0o022, 0o644, 0o755}, {0o002, 0o664, 0o775}} {
		t.Run(fmt.Sprintf("umask %03o", c.umask), func(t *testing.T) {
			old := syscall.Umask(c.umask)
			t.Cleanup(func() { syscall.Umask(old) })
			check := func(p string, want fs.FileMode) {
				t.Helper()
				fi, err := os.Stat(p)
				if err != nil {
					t.Fatal(err)
				}
				if got := fi.Mode().Perm(); got != want {
					t.Fatalf("%s is %04o, want %04o", p, got, want)
				}
			}

			dest := t.TempDir()
			first := put(t, t.TempDir(), "rel.rar", rar5Volume(false, spec{name: "Rel/movie.mkv", data: movie}))
			if _, err := Extract(first, dest); err != nil {
				t.Fatal(err)
			}
			check(filepath.Join(dest, "Rel", "movie.mkv"), c.file)
			check(filepath.Join(dest, "Rel"), c.dir)

			dest = t.TempDir()
			if err := ExtractCompressed(context.Background(), rar5First(), dest, rar5Size); err != nil {
				t.Fatal(err)
			}
			check(filepath.Join(dest, rar5Name), c.file)
		})
	}
}
