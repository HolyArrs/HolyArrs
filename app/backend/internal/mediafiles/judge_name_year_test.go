package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// The show-year rule stops wrong grabs only: a file already on disk or a
// finished download, named with an older year, still matches its show.
func TestShowYearRuleOnlyWhenChoosing(t *testing.T) {
	show := []TitleRef{{ID: 1, Name: "Show Name", Year: 2019, Kind: "show"}}
	file := releasename.Parse("Show Name (2015) - S01E01.mkv", releasename.Hints{Show: true})
	if file.Year.Value != 2015 {
		t.Fatalf("file year = %d, want 2015", file.Year.Value)
	}
	if m := MatchTitle(file, show); m.Title == nil {
		t.Errorf("import: file with an older year not matched: %s", m.Detail)
	}
	flash := []TitleRef{{ID: 2, Name: "The Flash", Year: 2014, Kind: "show"}}
	old := releasename.Parse("The.Flash.1990.S01E01.1080p.BluRay.x264-GRP", releasename.Hints{Show: true})
	if m := MatchRelease(old, flash); m.Title != nil {
		t.Errorf("search: The.Flash.1990 accepted for The Flash (2014): %s", m.Detail)
	}
	if m := MatchTitle(old, flash); m.Title == nil {
		t.Errorf("import: The.Flash.1990 file not matched to The Flash: %s", m.Detail)
	}
	if m := MatchRelease(releasename.Parse("The.Flash.2023.S09E01.1080p.WEB.h264-GRP", releasename.Hints{Show: true}), flash); m.Title != nil {
		t.Errorf("search: The.Flash.2023 accepted for The Flash (2014): %s", m.Detail)
	}
	for _, rel := range []string{"The.Flash.2014.S01E01.1080p.WEB.h264-GRP", "The.Flash.S09E01.2023.1080p.WEB.h264-GRP"} {
		if m := MatchRelease(releasename.Parse(rel, releasename.Hints{Show: true}), flash); m.Title == nil {
			t.Errorf("search: %s refused: %s", rel, m.Detail)
		}
	}
}

// Letters written apart that start with an A are an acronym, not an article.
func TestLettersApartStartingWithA(t *testing.T) {
	S, M := "show", "movie"
	for _, c := range []nameCase{
		{"ALF", 0, S, "A.L.F.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"A.L.F.", 0, S, "ALF.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"A.L.F.", 0, S, "A.L.F.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"A.P. Bio", 0, S, "A.P.Bio.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"A.P. Bio", 0, S, "AP.Bio.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"A Man Called Otto", 2022, M, "A.Man.Called.Otto.2022.1080p.WEB-DL.x264-GRP", true, "", ""},
		{"A Man Called Otto", 2022, M, "Man.Called.Otto.2022.1080p.WEB-DL.x264-GRP", true, "", ""},
		{"The A-Team", 2010, M, "The.A-Team.2010.1080p.BluRay.x264-GRP", true, "", ""},
		{"The A-Team", 2010, M, "The.A.Team.2010.1080p.BluRay.x264-GRP", true, "", ""},
		{"ALF", 0, S, "Alfie.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"ALF", 0, S, "L.F.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"A.L.F.", 0, S, "LF.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"The A-Team", 2010, M, "Team.2010.1080p.BluRay.x264-GRP", false, "", ""},
	} {
		p := releasename.Parse(c.rel, releasename.Hints{Show: c.kind == "show"})
		m := MatchRelease(p, []TitleRef{{ID: 1, Name: c.lib, Year: c.year, Kind: c.kind}})
		if got := m.Title != nil; got != c.want {
			t.Errorf("lib=%q rel=%q want=%v got=%v parsed=%q detail=%q", c.lib, c.rel, c.want, got, p.Title.Value, m.Detail)
		}
	}
}
