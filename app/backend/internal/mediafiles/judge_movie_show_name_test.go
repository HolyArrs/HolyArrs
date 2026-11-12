package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// feedMatch reads a name the way a feed check does: as named, then as a show.
func feedMatch(name string, lib []TitleRef) *TitleRef {
	m := MatchTitle(releasename.Parse(name, releasename.Hints{}), lib)
	if m.Title == nil {
		m = MatchTitle(releasename.Parse(name, releasename.Hints{Show: true}), lib)
	}
	return m.Title
}

// TestMatchTitle_ShowSharesAMovieName keeps a show and a movie of the same
// name apart, and keeps a sequel's number from reading as an episode.
func TestMatchTitle_ShowSharesAMovieName(t *testing.T) {
	show := TitleRef{ID: 1, Name: "Cabin in the Woods", Year: 2024, Kind: "show"}
	movie := TitleRef{ID: 2, Name: "The Cabin in the Woods", Year: 2011, Kind: "movie"}
	both := []TitleRef{show, movie}
	for name, want := range map[string]int64{
		"Cabin.in.the.Woods.S01.1080p.AMZN.WEB-DL.DDP2.0.H.264-Kitsune": 1,
		"The.Cabin.in.the.Woods.2011.1080p.BluRay.x264-GRP":             2,
	} {
		if got := feedMatch(name, both); got == nil || got.ID != want {
			t.Errorf("%s matched %+v, want title %d", name, got, want)
		}
	}
	if got := feedMatch("Cabin.in.the.Woods.2.2014.1080p.BluRay.x264-GRP", []TitleRef{show}); got != nil {
		t.Errorf("sequel movie matched the show: %+v", got)
	}
}
