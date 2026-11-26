package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A File whose name names no group is scored with the group stored on it,
// in its points, the search's current copy and good enough. A group the
// name does name wins over the stored one.
func TestOwnedFileReadsStoredGroup(t *testing.T) {
	SetOwnTraits([]OwnTrait{{Meta: database.Meta{ID: 1}, Name: "HONE", Source: "import", Specs: []OwnSpec{{Name: "HONE", Kind: "group", Value: "^HONE$", Required: true}}}})
	t.Cleanup(func() { SetOwnTraits(nil) })
	rules := GoodEveryday()
	hundred := 100
	rules.Traits = []TraitPoints{{Trait: OwnTraitKey(1), Points: 100}}
	rules.GoodEnoughPoints = &hundred
	name := "The Lord of the Rings - The Rings of Power (2022) S02E02 (1080p AMZN WEB-DL H265 SDR DDP Atmos 5.1 English - HONE)"
	hone, other := "HONE", "OTHER"
	for _, c := range []struct {
		why    string
		name   string
		group  *string
		points int
	}{
		{"no stored group", name, nil, 0},
		{"stored group", name, &hone, 100},
		{"the name's own group", "Show.S01E01.1080p.WEB.h264-HONE", &other, 100},
		{"the name's other group", "Show.S01E01.1080p.WEB.h264-NTb", &hone, 0},
	} {
		f := File{RelativePath: c.name + ".mkv", Group: c.group}
		if got := ScoreOwnedFile(f, c.name, nil, Title{}, rules).Points; got != c.points {
			t.Errorf("%s: %d points, want %d", c.why, got, c.points)
		}
		if got := CurrentOfOwnedFile(f, c.name, nil, Title{}, rules).Points; got != c.points {
			t.Errorf("%s: current %d points, want %d", c.why, got, c.points)
		}
		if got := OwnedFileReachesGoodEnough(f, c.name, Title{}, rules); got != (c.points >= 100) {
			t.Errorf("%s: good enough %v", c.why, got)
		}
	}
	// The forms without a File read the name alone, as before.
	if got := ScoreTitleFile(name, 0, nil, Title{}, rules).Points; got != 0 {
		t.Errorf("ScoreTitleFile %d", got)
	}
}
