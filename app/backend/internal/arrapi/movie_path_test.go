package arrapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A flat movie (its file loose in the Library folder) never reports the
// Library folder as its path, so a tool that deletes movie.path cannot
// take every other movie with it.
func TestMoviePath_NeverTheLibraryRoot(t *testing.T) {
	root := filepath.FromSlash("/movies")
	for _, c := range []struct {
		name, folder, title string
		year                int
		want                string
	}{
		{"own folder", "Alien (1979)", "Alien", 1979, "/movies/Alien (1979)"},
		{"flat", "", "Alien", 1979, "/movies/Alien (1979)"},
		{"flat no year", "", "Alien", 0, "/movies/Alien"},
		{"flat odd name", "", `AC/DC: Live?`, 1991, "/movies/AC DC Live (1991)"},
		{"flat empty name", "", " ", 0, "/movies/Movie 7"},
		{"flat dots", "", "..", 0, "/movies/Movie 7"},
		{"folder climbs out", "..", "Alien", 1979, "/movies/Alien (1979)"},
		{"folder is dot", ".", "Alien", 1979, "/movies/Alien (1979)"},
	} {
		got := moviePath(root, c.folder, 7, c.title, c.year)
		if got != filepath.FromSlash(c.want) {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
		if filepath.Clean(got) == filepath.Clean(root) {
			t.Errorf("%s: path is the Library folder", c.name)
		}
	}
}

// A fake cleanup tool deletes movie.path from disk for a flat movie. The
// other movies in the flat folder stay, and movieFile.path still names
// the real file.
func TestMoviePath_FakeClientDeletesPath(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"Alien (1979).mkv", "Heat (1995).mkv", "Alien (1979).en.srt"} {
		if err := os.WriteFile(filepath.Join(root, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := moviePath(root, "", 1, "Alien", 1979)
	if err := os.RemoveAll(p); err != nil { // what the outside tool does
		t.Fatal(err)
	}
	for _, n := range []string{"Alien (1979).mkv", "Heat (1995).mkv", "Alien (1979).en.srt"} {
		if _, err := os.Stat(filepath.Join(root, n)); err != nil {
			t.Errorf("%s gone after deleting movie.path: %v", n, err)
		}
	}
	f := mediafiles.File{RelativePath: "Alien (1979).mkv", AddedAt: time.Now()}
	mf := mediaFileOf("radarr", f, root, "")
	if mf.Path != filepath.Join(root, "Alien (1979).mkv") || mf.RelativePath != "Alien (1979).mkv" {
		t.Errorf("movie file: %+v", mf)
	}
}
