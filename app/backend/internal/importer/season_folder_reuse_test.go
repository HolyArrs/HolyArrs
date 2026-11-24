package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func TestExistingSeasonFolder(t *testing.T) {
	cases := []struct {
		name   string
		dirs   []string
		season int
		named  string
		want   string
	}{
		{"none, use setting", nil, 1, "Season 1", "Season 1"},
		{"padded exists", []string{"Season 01"}, 1, "Season 1", "Season 01"},
		{"three digits", []string{"Season 001"}, 1, "Season 1", "Season 001"},
		{"lower case", []string{"season 1"}, 1, "Season 01", "season 1"},
		{"other season only", []string{"Season 02"}, 1, "Season 1", "Season 1"},
		{"season 10 not 1", []string{"Season 10"}, 1, "Season 1", "Season 1"},
		{"exact wins", []string{"Season 01", "Season 1"}, 1, "Season 1", "Season 1"},
		{"specials", []string{"specials"}, 0, "Specials", "specials"},
		{"season 00 for specials", []string{"Season 00"}, 0, "Specials", "Season 00"},
		{"custom pattern any case", []string{"s01 extra"}, 1, "S01 Extra", "s01 extra"},
		{"folders off", []string{"Season 01"}, 1, "", ""},
		{"file not dir", []string{}, 1, "Season 1", "Season 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, d := range c.dirs {
				if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if c.name == "file not dir" {
				os.WriteFile(filepath.Join(dir, "Season 01"), nil, 0o644)
			}
			if got := ExistingSeasonFolder(dir, c.season, c.named); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
	if got := ExistingSeasonFolder(filepath.Join(t.TempDir(), "missing"), 1, "Season 1"); got != "Season 1" {
		t.Fatalf("missing show dir: %q", got)
	}
}

func TestEpisodePathReusesSeasonFolder(t *testing.T) {
	lib := t.TempDir()
	os.MkdirAll(filepath.Join(lib, "Show (2020)", "Season 01"), 0o755)
	tg := ShowTarget{LibraryPath: lib, FolderName: "Show (2020)"}
	tg.Show.Name, tg.Show.Year = "Show", 2020
	e := Episode{}
	e.Ref.Season, e.Ref.Episode = 1, 2
	p := tg.episodePath(releasename.Parse("Show.2020.S01E02.1080p.WEB-DL.x264-GRP", releasename.Hints{Show: true}), "Show.2020.S01E02.1080p.WEB-DL.x264-GRP.mkv", []Episode{e})
	if filepath.Dir(filepath.FromSlash(p)) != filepath.Join("Show (2020)", "Season 01") {
		t.Fatalf("path %q", p)
	}
}
