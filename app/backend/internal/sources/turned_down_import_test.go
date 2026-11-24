package sources

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// An imported release is turned down only for the episodes it was for:
// the old app's exact list first, else what its name gives. A show name
// with no episode never turns down the whole show.
func TestMatchNamedSlots(t *testing.T) {
	all := []mediafiles.NumberedSlot{{ID: 1, Season: 1, Episode: 1, Absolute: 1, AirDate: "2024-01-04"}, {ID: 2, Season: 1, Episode: 2, Absolute: 2, AirDate: "2024-01-05"},
		{ID: 3, Season: 2, Episode: 1, Absolute: 3}, {ID: 4, Season: 2, Episode: 2}, {ID: 5, Season: 0, Episode: 1}}
	for _, c := range []struct {
		name  string
		exact []EpisodeNumber
		want  []int64
	}{
		{"Lab.Show.S01E02.1080p.WEB-GRP", nil, []int64{2}},
		{"Lab.Show.S02.1080p.BluRay-GRP", nil, []int64{3, 4}},
		{"Lab Show S01E01E02 720p HDTV", nil, []int64{1, 2}},
		{"Lab.Show.S05E01.1080p-GRP", nil, nil},
		{"[Grp] Lab Show - 02 [1080p]", nil, []int64{2}},
		{"Lab.Show.2024.01.05.1080p.WEB-GRP", nil, []int64{2}},
		{"Some.Movie.2020.1080p.WEBRip-LAMA", nil, nil},
		{"", nil, nil},
		// Exact episodes beat the name, even a misleading one.
		{"[Grp] Lab Show - 02 [1080p]", []EpisodeNumber{{1, 1}}, []int64{1}},
		{"Lab.Show.S01E02.1080p.WEB-GRP", []EpisodeNumber{{2, 1}, {2, 2}}, []int64{3, 4}},
		{"Lab.Show.S01E02.1080p.WEB-GRP", []EpisodeNumber{{9, 9}}, nil},
	} {
		if got := matchNamedSlots(c.name, c.exact, all); !slices.Equal(got, c.want) {
			t.Errorf("%q %v: %v, want %v", c.name, c.exact, got, c.want)
		}
	}
	movie := []mediafiles.NumberedSlot{{ID: 9}, {ID: 10}}
	if got := matchNamedSlots("Some.Movie.2020.1080p.WEBRip-LAMA", nil, movie); !slices.Equal(got, []int64{9, 10}) {
		t.Errorf("movie %v", got)
	}
}
