package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A release named with a local name of a non-anime movie or show finds its
// Title; the year check still applies; a name no Title carries finds none.
func TestMatchTitleByLocalName(t *testing.T) {
	lib := []TitleRef{
		{ID: 1, Name: "Money Heist", Year: 2017, Kind: "show", Aliases: []string{"La Casa de Papel", "Haus des Geldes"}},
		{ID: 2, Name: "Amelie", Year: 2001, Kind: "movie", Aliases: []string{"Le Fabuleux Destin d'Amélie Poulain"}},
	}
	cases := []struct {
		name string
		want int64
	}{
		{"La.Casa.de.Papel.S01E01.1080p.WEB-DL.DDP5.1.H.264-GRP", 1},
		{"Money.Heist.S01E01.1080p.WEB-DL.H.264-GRP", 1},
		{"Le.Fabuleux.Destin.d.Amelie.Poulain.2001.1080p.BluRay.x264-GRP", 2},
		{"Le.Fabuleux.Destin.d.Amelie.Poulain.1999.1080p.BluRay.x264-GRP", 0},
		{"La.Casa.de.Otro.S01E01.1080p.WEB-DL.H.264-GRP", 0},
	}
	for _, c := range cases {
		m := MatchTitle(releasename.Parse(c.name, releasename.Hints{}), lib)
		var got int64
		if m.Title != nil {
			got = m.Title.ID
		}
		if got != c.want {
			t.Errorf("%s: matched Title %d, want %d (%s)", c.name, got, c.want, m.Detail)
		}
	}
}
