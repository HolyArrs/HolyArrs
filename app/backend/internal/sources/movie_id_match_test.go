package sources

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestSameMovieID(t *testing.T) {
	ti := mediafiles.Title{SourceIDs: map[string]string{"tmdb": "1734", "imdb": "tt0209163"}}
	for _, c := range []struct {
		it   Item
		want bool
	}{
		{Item{TMDb: 1734}, true},
		{Item{IMDb: "209163"}, true},
		{Item{TMDb: 99}, false},
		{Item{IMDb: "1"}, false},
		{Item{}, false},
	} {
		if got := sameMovieID(ti, c.it); got != c.want {
			t.Errorf("sameMovieID(%+v) = %v, want %v", c.it, got, c.want)
		}
	}
	if sameMovieID(mediafiles.Title{}, Item{}) {
		t.Error("no ids on either side matched")
	}
}
