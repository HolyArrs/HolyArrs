package sources

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// An imported release is turned down only for the episodes its name gives.
func TestMatchNamedSlots(t *testing.T) {
	all := []mediafiles.NumberedSlot{{ID: 1, Season: 1, Episode: 1}, {ID: 2, Season: 1, Episode: 2}, {ID: 3, Season: 2, Episode: 1}, {ID: 4, Season: 2, Episode: 2}}
	for name, want := range map[string][]int64{
		"Lab.Show.S01E02.1080p.WEB-GRP":     {2},
		"Lab.Show.S02.1080p.BluRay-GRP":     {3, 4},
		"Lab Show S01E01E02 720p HDTV":      {1, 2},
		"Some.Movie.2020.1080p.WEBRip-LAMA": {1, 2, 3, 4},
	} {
		if got := matchNamedSlots(name, all); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
	movie := []mediafiles.NumberedSlot{{ID: 9}}
	if got := matchNamedSlots("Some.Movie.2020.1080p.WEBRip-LAMA", movie); !slices.Equal(got, []int64{9}) {
		t.Errorf("movie %v", got)
	}
}
