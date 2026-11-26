package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// "Local title / Original title [Year, ...]" (RuTracker) matches on either
// side of the slash, even when the Title does not know the local name.
func TestMatchTitleSlashNames(t *testing.T) {
	lib := []TitleRef{
		{ID: 1, Name: "Pirates of the Caribbean: The Curse of the Black Pearl", Year: 2003, Kind: "movie"},
		{ID: 2, Name: "Брат", Year: 1997, Kind: "movie", Aliases: []string{"Brother"}},
		{ID: 3, Name: "Pirates", Year: 2003, Kind: "movie"},
	}
	cases := []struct {
		name string
		want int64
	}{
		{"Пираты Карибского моря: Проклятие Черной жемчужины / Pirates of the Caribbean: The Curse of the Black Pearl [2003, США, приключения, WEB-DL 2160p, HDR10, HDR10+, Dolby Vision] [Hybrid] Dub + 2x MVO + Original Eng", 1},
		{"Pirates of the Caribbean: The Curse of the Black Pearl [2003, WEB-DL 2160p, HDR10, HDR10+, Dolby Vision] [Hybrid] Dub + 2x MVO + RUS", 1},
		{"Брат / Brother [1997, Россия, драма, BDRip 1080p] MVO", 2},
		{"Неизвестный / Unknown Film [2003, США, BDRip 1080p] MVO", 0},
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
