package library

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindFolder437 covers the folder lookup an add does: id tags,
// letter folders spelled in other case, other names, one year off, and
// ambiguity that must not guess.
func TestFindFolder437(t *testing.T) {
	ids := map[string]string{"tmdb": "771", "imdb": "tt0099785"}
	for _, c := range []struct {
		name string
		dirs []string
		want string
		f    FolderFacts
		exp  string
	}{
		{"id tag wins over other name", []string{"Kevin (1990) {tmdb-771}"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990, IDs: ids}, "Kevin (1990) {tmdb-771}"},
		{"imdb tag", []string{"Whatever [imdbid-tt0099785]"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990, IDs: ids}, "Whatever [imdbid-tt0099785]"},
		{"other title's id not taken", []string{"Home Alone (1990) {tmdb-999}"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990, IDs: ids}, "Home Alone (1990)"},
		{"lower-case letter folder", []string{"m/Home Alone (1990)"}, "M/Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990}, "m/Home Alone (1990)"},
		{"letter folder, renamed title", []string{"h/home alone (1990)"}, "H/Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990}, "h/home alone (1990)"},
		{"original name", []string{"Le Fabuleux Destin d'Amélie Poulain (2001)"}, "Amelie (2001)", FolderFacts{Name: "Amelie", Year: 2001, Other: []string{"Le Fabuleux Destin d'Amélie Poulain"}}, "Le Fabuleux Destin d'Amélie Poulain (2001)"},
		{"one year off", []string{"Home Alone (1991)"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990}, "Home Alone (1991)"},
		{"two years off is not it", []string{"Home Alone (1992)"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990}, "Home Alone (1990)"},
		{"exact year beats one off", []string{"Home Alone (1991)", "home alone (1990)"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990}, "home alone (1990)"},
		{"two one-off: no guess", []string{"Home Alone (1989)", "Home Alone (1991)"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990}, "Home Alone (1990)"},
		{"two same ids: no guess", []string{"A {tmdb-771}", "B {tmdb-771}"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990, IDs: ids}, "Home Alone (1990)"},
		{"case only, no letter folder", []string{"HOME ALONE (1990)"}, "Home Alone (1990)", FolderFacts{Name: "Home Alone", Year: 1990}, "HOME ALONE (1990)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for _, d := range c.dirs {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			got := FindFolder(root, c.want, c.f, nil)
			if FoldsCase(root) && len(got) == len(c.exp) && got != c.exp {
				// On a case-folding disk the spelling asked for opens the same folder.
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(got))); err == nil && sameFolderName(got, c.exp, true) {
					return
				}
			}
			if got != c.exp {
				t.Fatalf("got %q, want %q", got, c.exp)
			}
		})
	}
}

// TestFindFolderTaken: a folder another title already uses is skipped,
// keyed the way the disk compares names.
func TestFindFolderTaken(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Home Alone (1990)"), 0o755)
	got := FindFolder(root, "Home Alone (1990) x", FolderFacts{Name: "Home Alone", Year: 1990}, func(d string) bool { return d == "Home Alone (1990)" })
	if got != "Home Alone (1990) x" {
		t.Fatalf("got %q", got)
	}
}
