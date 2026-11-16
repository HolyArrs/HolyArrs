package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func TestMatchTitleCountryAndYearTag(t *testing.T) {
	cases := []struct {
		name string
		lib  []TitleRef
		rel  string
		id   int64 // 0: must not match
	}{
		{"us plain", []TitleRef{{ID: 1, Kind: "show", Name: "The Office (US)", Year: 2005}}, "The.Office.S01E01.1080p.WEB-DL", 1},
		{"us tagged", []TitleRef{{ID: 1, Kind: "show", Name: "The Office (US)", Year: 2005}}, "The.Office.US.S01E01.1080p.WEB-DL", 1},
		{"us refuses uk", []TitleRef{{ID: 1, Kind: "show", Name: "The Office (US)", Year: 2005}}, "The.Office.UK.S01E01.1080p.WEB-DL", 0},
		{"uk plain", []TitleRef{{ID: 2, Kind: "show", Name: "The Office (UK)", Year: 2001}}, "The.Office.S01E01.720p.HDTV", 2},
		{"uk tagged", []TitleRef{{ID: 2, Kind: "show", Name: "The Office (UK)", Year: 2001}}, "The.Office.UK.S01E01.720p.HDTV", 2},
		{"uk refuses us", []TitleRef{{ID: 2, Kind: "show", Name: "The Office (UK)", Year: 2001}}, "The.Office.US.S01E01.720p.HDTV", 0},
		{"au tagged", []TitleRef{{ID: 3, Kind: "show", Name: "Gogglebox (AU)"}}, "Gogglebox.AU.S01E01.720p.HDTV", 3},
		{"au plain", []TitleRef{{ID: 3, Kind: "show", Name: "Gogglebox (AU)"}}, "Gogglebox.S01E01.720p.HDTV", 3},
		{"au refuses uk", []TitleRef{{ID: 3, Kind: "show", Name: "Gogglebox (AU)"}}, "Gogglebox.UK.S01E01.720p.HDTV", 0},
		{"nz tagged", []TitleRef{{ID: 4, Kind: "show", Name: "The Block (NZ)"}}, "The.Block.NZ.S01E01.720p.HDTV", 4},
		{"nz refuses au", []TitleRef{{ID: 4, Kind: "show", Name: "The Block (NZ)"}}, "The.Block.AU.S01E01.720p.HDTV", 0},
		{"ca tagged", []TitleRef{{ID: 5, Kind: "show", Name: "Big Brother (CA)"}}, "Big.Brother.CA.S01E01.720p.HDTV", 5},
		{"ca plain", []TitleRef{{ID: 5, Kind: "show", Name: "Big Brother (CA)"}}, "Big.Brother.S01E01.720p.HDTV", 5},
		{"ca refuses us", []TitleRef{{ID: 5, Kind: "show", Name: "Big Brother (CA)"}}, "Big.Brother.US.S01E01.720p.HDTV", 0},
		{"year tagged", []TitleRef{{ID: 6, Kind: "show", Name: "Doctor Who (2005)", Year: 2005}}, "Doctor.Who.2005.S01E01.720p.HDTV", 6},
		{"year plain", []TitleRef{{ID: 6, Kind: "show", Name: "Doctor Who (2005)", Year: 2005}}, "Doctor.Who.S01E01.720p.HDTV", 6},
		{"year refuses other year", []TitleRef{{ID: 6, Kind: "show", Name: "Doctor Who (2005)", Year: 2005}}, "Doctor.Who.1963.S01E01.480p.DVDRip", 0},
		{"two offices, plain name is ambiguous", []TitleRef{
			{ID: 1, Kind: "show", Name: "The Office (US)", Year: 2005},
			{ID: 2, Kind: "show", Name: "The Office (UK)", Year: 2001},
		}, "The.Office.S01E01.1080p.WEB-DL", 0},
		{"two offices, tag picks", []TitleRef{
			{ID: 1, Kind: "show", Name: "The Office (US)", Year: 2005},
			{ID: 2, Kind: "show", Name: "The Office (UK)", Year: 2001},
		}, "The.Office.UK.S01E01.1080p.WEB-DL", 2},
		{"exact name wins over tagged", []TitleRef{
			{ID: 7, Kind: "show", Name: "The Office"},
			{ID: 1, Kind: "show", Name: "The Office (US)", Year: 2005},
		}, "The.Office.S01E01.1080p.WEB-DL", 7},
		{"two doctor whos, plain is ambiguous", []TitleRef{
			{ID: 6, Kind: "show", Name: "Doctor Who (2005)", Year: 2005},
			{ID: 8, Kind: "show", Name: "Doctor Who (1963)", Year: 1963},
		}, "Doctor.Who.S01E01.720p.HDTV", 0},
		{"year picks tagged remake over plain original", []TitleRef{
			{ID: 1, Kind: "show", Name: "Doctor Who", Year: 1963},
			{ID: 6, Kind: "show", Name: "Doctor Who (2005)", Year: 2005},
		}, "Doctor.Who.2005.S01E01.720p.HDTV", 6},
		{"year picks plain original over tagged remake", []TitleRef{
			{ID: 1, Kind: "show", Name: "Doctor Who", Year: 1963},
			{ID: 6, Kind: "show", Name: "Doctor Who (2005)", Year: 2005},
		}, "Doctor.Who.1963.S01E01.480p.DVDRip", 1},
		{"tag in alias", []TitleRef{{ID: 9, Kind: "show", Name: "Other", Aliases: []string{"Shameless (US)"}}}, "Shameless.S01E01.720p.HDTV", 9},
		{"other tag text kept", []TitleRef{{ID: 10, Kind: "show", Name: "Ghosts (Remix)"}}, "Ghosts.S01E01.720p.HDTV", 0},
		{"movie unaffected", []TitleRef{{ID: 11, Kind: "movie", Name: "The Office (US)", Year: 2005}}, "The.Office.2005.1080p.BluRay", 0},
	}
	for _, c := range cases {
		show := c.lib[0].Kind == "show"
		p := releasename.Parse(c.rel, releasename.Hints{Show: show})
		m := MatchTitle(p, c.lib)
		switch {
		case c.id == 0 && m.Title != nil:
			t.Errorf("%s: %s took %s", c.name, c.rel, m.Title.Name)
		case c.id != 0 && (m.Title == nil || m.Title.ID != c.id):
			t.Errorf("%s: %s got %+v", c.name, c.rel, m)
		}
	}
}
