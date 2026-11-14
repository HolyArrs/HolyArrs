package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func TestMatchTitleApostrophe(t *testing.T) {
	lib := []TitleRef{
		{ID: 1, Kind: "show", Name: "Schitt's Creek"},
		{ID: 2, Kind: "movie", Name: "Ocean's Eleven", Year: 2001},
		{ID: 3, Kind: "show", Name: "Other", Aliases: []string{"Grey’s Story"}},
		{ID: 4, Kind: "movie", Name: "L'Odyssée d'Amélie", Year: 2010},
		{ID: 5, Kind: "artist", Name: "Guns N' Roses"},
	}
	cases := []struct {
		rel  string
		show bool
		id   int64
	}{
		{"Schitts.Creek.S04E02.1080p.WEB-DL", true, 1},
		{"Schitt.s.Creek.S04E02.1080p.WEB-DL", true, 1},
		{"Schitt's.Creek.S04E02.1080p.WEB-DL", true, 1},
		{"Oceans.Eleven.2001.1080p.BluRay", false, 2},
		{"Ocean.s.Eleven.2001.1080p.BluRay", false, 2},
		{"Greys.Story.S01E01.1080p.WEB-DL", true, 3},
		{"L.Odyssee.d.Amelie.2010.1080p.BluRay", false, 4},
		{"LOdyssee.dAmelie.2010.1080p.BluRay", false, 4},
	}
	for _, c := range cases {
		p := releasename.Parse(c.rel, releasename.Hints{Show: c.show})
		m := MatchTitle(p, lib)
		if m.Title == nil || m.Title.ID != c.id {
			t.Errorf("%s: got %+v", c.rel, m)
		}
	}
	p := releasename.Parse("Guns N Roses - Appetite (1987) FLAC", releasename.Hints{Music: true})
	if m := MatchTitle(p, lib); m.Title == nil || m.Title.ID != 5 {
		t.Errorf("artist: %+v", m)
	}
}
