package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// TestSizeTrait: a size trait fits over its low end and up to and
// including its high end; an unknown size never fits.
func TestSizeTrait(t *testing.T) {
	t.Cleanup(func() { SetOwnTraits(nil) })
	SetOwnTraits([]OwnTrait{
		{Meta: database.Meta{ID: 1}, Name: "Huge", Match: "size", Terms: []string{"60-"}},
		{Meta: database.Meta{ID: 2}, Name: "Small", Match: "size", Terms: []string{"-8"}},
		{Meta: database.Meta{ID: 3}, Name: "Middle", Match: "size", Terms: []string{"2-8"}},
		{Meta: database.Meta{ID: 4}, Name: "Synced", Match: "name", Specs: []OwnSpec{{Name: "Size", Kind: "size", Value: "2-8"}}},
	})
	const gb = int64(1 << 30)
	cases := []struct {
		id   int64
		size int64
		want bool
	}{
		{1, 61 * gb, true},
		{1, 60 * gb, false},
		{1, 0, false},
		{2, 8 * gb, true},
		{2, 8*gb + 1, false},
		{2, 0, false},
		{3, 2 * gb, false},
		{3, 5 * gb, true},
		{3, 8 * gb, true},
		{3, 9 * gb, false},
		{3, 0, false},
		{4, 5 * gb, true},
		{4, 9 * gb, false},
		{4, 0, false},
	}
	for _, c := range cases {
		p := releasename.Parse("Movie.2024.1080p.WEB-DL.H264-GOODA", releasename.Hints{})
		p.Size = c.size
		if got, why := ownMatches(c.id, p); got != c.want {
			t.Errorf("trait %d at %d bytes = %v (%s), want %v", c.id, c.size, got, why, c.want)
		}
	}
}

func TestCheckSizeTrait(t *testing.T) {
	ok := []string{"60-", " -8 ", "2-8", "1.5-"}
	for _, v := range ok {
		tr := OwnTrait{Name: "S", Match: "size", Terms: []string{v}}
		if err := CheckOwnTrait(&tr); err != nil {
			t.Errorf("%q: %v", v, err)
		}
	}
	for _, v := range []string{"-", "8-2", "x-", "5", "-0"} {
		tr := OwnTrait{Name: "S", Match: "size", Terms: []string{v}}
		if err := CheckOwnTrait(&tr); err == nil {
			t.Errorf("%q was taken", v)
		}
	}
}
