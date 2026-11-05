//go:build unix

package archive

import (
	"archive/zip"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestUnpackedModesFollowTheUmask(t *testing.T) {
	for _, c := range []struct {
		umask     int
		file, dir fs.FileMode
	}{{0o022, 0o644, 0o755}, {0o002, 0o664, 0o775}} {
		t.Run(fmt.Sprintf("umask %03o", c.umask), func(t *testing.T) {
			old := syscall.Umask(c.umask)
			t.Cleanup(func() { syscall.Umask(old) })
			src, dest := t.TempDir(), t.TempDir()
			name := filepath.Join(src, "rel.zip")
			f, err := os.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(f)
			if _, err := zw.Create("Rel/Extras/"); err != nil {
				t.Fatal(err)
			}
			w, err := zw.Create("Rel/movie.mkv")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("movie")); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if err := Extract(context.Background(), name, Zip, dest, 1<<20); err != nil {
				t.Fatal(err)
			}
			for p, want := range map[string]fs.FileMode{
				filepath.Join(dest, "Rel", "movie.mkv"): c.file,
				filepath.Join(dest, "Rel"):              c.dir,
				filepath.Join(dest, "Rel", "Extras"):    c.dir,
			} {
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
