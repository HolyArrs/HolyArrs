package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A Release name with "&amp;" left as "amp", or with the "&" dropped, still
// finds a library name that has an "&".
func TestNameShapesAmpersand(t *testing.T) {
	lib := []TitleRef{{ID: 1, Name: "New PANTY & STOCKING with GARTERBELT", Year: 2025, Kind: "show"}}
	for _, rel := range []string{
		"New Panty  amp  Stocking with Garterbelt S01E01 1080p WEB h264-GRP",
		"New Panty Stocking with Garterbelt S01E01 1080p WEB h264-GRP",
		"[DBD Raws] New PANTY  amp  STOCKING with GARTERBELT - 05 [1080p]",
	} {
		p := releasename.Parse(rel, releasename.Hints{Show: true})
		if m := MatchTitle(p, lib); m.Title == nil {
			t.Errorf("%q: no match; parsed title %q; %s", rel, p.Title.Value, m.Detail)
		}
	}
}

// A real word "Amp" in a library name still matches a Release that has it,
// and the "amp" from a Release's "&amp;" reads as "&".
func TestNameShapesAmpWord(t *testing.T) {
	for _, c := range []struct {
		id, name, rel string
		show, want    bool
	}{
		{"A30", "Guitar Amp Hero", "Guitar.amp.Hero.2010.1080p.BluRay.x264-GRP", false, true},
		{"A31", "Guitar Amp Hero", "guitar amp hero 2010 1080p BluRay x264-GRP", false, true},
		{"A32", "Tube Amp Rock", "Tube amp Rock S01E01 1080p WEB h264-GRP", true, true},
		{"A33", "Law & Order", "Law amp Order S01E01 1080p WEB h264-GRP", true, true},
		{"A34", "Camp & Rock", "Camp amp Rock S01E01 1080p WEB h264-GRP", true, true},
		{"A35", "Camp Rock", "Camp amp Rock S01E01 1080p WEB h264-GRP", true, false},
		{"A36", "Law", "Law amp S01E01 1080p WEB h264-GRP", true, false},
		{"A37", "Amp", "Amp.2010.1080p.WEB-DL.x264-GRP", false, true},
		{"A38", "Tom & Jerry", "Tom Jerry S01E01 1080p WEB h264-GRP", true, true},
	} {
		kind := "movie"
		if c.show {
			kind = "show"
		}
		lib := []TitleRef{{ID: 1, Name: c.name, Year: 2010, Kind: kind}}
		p := releasename.Parse(c.rel, releasename.Hints{Show: c.show})
		if m := MatchTitle(p, lib); (m.Title != nil) != c.want {
			t.Errorf("%s %q vs %q: matched %v, want %v; parsed title %q", c.id, c.name, c.rel, m.Title != nil, c.want, p.Title.Value)
		}
	}
}
