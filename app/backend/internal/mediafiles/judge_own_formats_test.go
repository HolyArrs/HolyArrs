package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// TestSyncedTraitTests: per kind, required tests must all pass and one of
// the others must; negate turns a test around; look-behind patterns work.
func TestSyncedTraitTests(t *testing.T) {
	t.Cleanup(func() { SetOwnTraits(nil) })
	SetOwnTraits([]OwnTrait{
		{Meta: database.Meta{ID: 1}, Name: "HEVC not 4K", Match: "name", Specs: []OwnSpec{
			{Name: "x265", Kind: "name", Value: `[xh][ ._-]?265|\bHEVC\b`, Required: true},
			{Name: "Not 2160p", Kind: "resolution", Value: "2160", Negate: true, Required: true},
		}},
		{Meta: database.Meta{ID: 2}, Name: "Web", Match: "name", Specs: []OwnSpec{
			{Name: "WEB-DL", Kind: "source", Value: "web_dl"},
			{Name: "WEBRip", Kind: "source", Value: "web_rip"},
		}},
		{Meta: database.Meta{ID: 3}, Name: "Group", Match: "name", Specs: []OwnSpec{
			{Name: "Good", Kind: "group", Value: `^(GOODA|GOODB)$`},
		}},
		{Meta: database.Meta{ID: 4}, Name: "Lookbehind", Match: "name", Specs: []OwnSpec{
			{Name: "DV not HDR10", Kind: "name", Value: `(?<!HDR10[ ._-])\bDV\b`},
		}},
		{Meta: database.Meta{ID: 5}, Name: "Broken", Match: "name", Specs: []OwnSpec{{Name: "bad", Kind: "name", Value: `(`}}},
	})
	cases := []struct {
		id   int64
		name string
		want bool
	}{
		{1, "Show.S01E01.1080p.WEB-DL.x265-GOODA", true},
		{1, "Show.S01E01.2160p.WEB-DL.x265-GOODA", false},
		{1, "Show.S01E01.1080p.WEB-DL.H264-GOODA", false},
		{2, "Show.S01E01.1080p.WEBRip.H264-GOODA", true},
		{2, "Show.S01E01.1080p.BluRay.H264-GOODA", false},
		{3, "Show.S01E01.1080p.BluRay.H264-GOODB", true},
		{3, "Show.S01E01.1080p.BluRay.H264-OTHER", false},
		{4, "Movie.2024.2160p.WEB-DL.DV.H265-GOODA", true},
		{4, "Movie.2024.2160p.WEB-DL.HDR10.DV.H265-GOODA", false},
		{5, "Movie.2024.2160p.WEB-DL.H265-GOODA", false},
	}
	for _, c := range cases {
		got, why := ownMatches(c.id, releasename.Parse(c.name, releasename.Hints{}))
		if got != c.want {
			t.Errorf("trait %d on %s = %v (%s), want %v", c.id, c.name, got, why, c.want)
		}
	}
}
