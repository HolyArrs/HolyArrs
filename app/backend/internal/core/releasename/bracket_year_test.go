package releasename

import "testing"

// RuTracker names put the year first inside square brackets, followed by a
// comma: "Local / Original [2003, Country, Genre, Source] Audio".
func TestParseBracketYearFirst(t *testing.T) {
	cases := []struct {
		name, title string
		year, res   int
	}{
		{"Пираты Карибского моря: Проклятие Черной жемчужины / Pirates of the Caribbean: The Curse of the Black Pearl [2003, США, приключения, WEB-DL 2160p, HDR10, HDR10+, Dolby Vision] [Hybrid] Dub + 2x MVO + Original Eng",
			"Пираты Карибского моря: Проклятие Черной жемчужины / Pirates of the Caribbean: The Curse of the Black Pearl", 2003, 2160},
		{"Pirates of the Caribbean: The Curse of the Black Pearl [2003, WEB-DL 2160p, HDR10, HDR10+, Dolby Vision] [Hybrid] Dub + 2x MVO + RUS",
			"Pirates of the Caribbean: The Curse of the Black Pearl", 2003, 2160},
		{"Брат / Brother [1997, Россия, драма, BDRip 1080p] MVO",
			"Брат / Brother", 1997, 1080},
	}
	for _, c := range cases {
		r := Parse(c.name, Hints{})
		if r.Title.Value != c.title || r.Year.Value != c.year || r.Resolution.Value != c.res {
			t.Errorf("%s:\n got title %q year %d res %d\nwant title %q year %d res %d", c.name, r.Title.Value, r.Year.Value, r.Resolution.Value, c.title, c.year, c.res)
		}
	}
}
