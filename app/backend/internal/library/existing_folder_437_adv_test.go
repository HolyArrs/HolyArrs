package library

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindFolder437Edges: accented, localized, case-variant and id-carrying
// names the lookup must get right.
func TestFindFolder437Edges(t *testing.T) {
	for _, c := range []struct {
		name string
		dirs []string
		want string
		f    FolderFacts
		exp  string
	}{
		{"accents and case", []string{"les miserables (2012)"}, "Les Misérables (2012)", FolderFacts{Name: "Les Misérables", Year: 2012}, "les miserables (2012)"},
		{"french local name via naming", []string{"La Ligne Verte (1999)"}, "La Ligne verte (1999)", FolderFacts{Name: "The Green Mile", Year: 1999}, "La Ligne Verte (1999)"},
		{"number title", []string{"1917 (2019)"}, "1917 (2019)", FolderFacts{Name: "1917", Year: 2019}, "1917 (2019)"},
		{"year-like name", []string{"2001 A Space Odyssey (1968)"}, "2001 - A Space Odyssey (1968)", FolderFacts{Name: "2001: A Space Odyssey", Year: 1968}, "2001 A Space Odyssey (1968)"},
		{"number in name not a year", []string{"Blade Runner 2049 (2017)"}, "Blade Runner 2049 (2017) {tmdb-335984}", FolderFacts{Name: "Blade Runner 2049", Year: 2017, IDs: map[string]string{"tmdb": "335984"}}, "Blade Runner 2049 (2017)"},
		{"id prefix is not the id", []string{"Amélie {tmdb-1940}"}, "Amélie (2001)", FolderFacts{Name: "Amélie", Year: 2001, IDs: map[string]string{"tmdb": "194"}}, "Amélie (2001)"},
		{"id tag beats other year", []string{"Amelie (2002) [tmdbid-194]", "Amelie (2001)"}, "Amélie (2001)", FolderFacts{Name: "Amélie", Year: 2001, IDs: map[string]string{"tmdb": "194"}}, "Amelie (2002) [tmdbid-194]"},
		{"accented letter folder", []string{"é/Été (2020)"}, "É/Été (2020)", FolderFacts{Name: "Été", Year: 2020}, "é/Été (2020)"},
		{"other year, other film", []string{"Dune (1984)"}, "Dune (2021)", FolderFacts{Name: "Dune", Year: 2021}, "Dune (2021)"},
		{"sequel is not it", []string{"Aliens (1986)"}, "Alien (1979)", FolderFacts{Name: "Alien", Year: 1979}, "Alien (1979)"},
		{"empty alias ignored", []string{"Other (1999)"}, "Matrix (1999)", FolderFacts{Name: "Matrix", Year: 1999, Other: []string{"", "  "}}, "Matrix (1999)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for _, d := range c.dirs {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			got := FindFolder(root, c.want, c.f, nil)
			if got != c.exp && !(FoldsCase(root) && sameFolderName(got, c.exp, true)) {
				t.Fatalf("got %q, want %q", got, c.exp)
			}
		})
	}
}
