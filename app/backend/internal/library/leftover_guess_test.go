package library

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func ep(season, number int, name string) mediafiles.Item {
	return mediafiles.Item{Kind: "episode", Numbers: mediafiles.Numbers{Season: &season, Episode: &number}, Name: &name}
}

// A file whose seasons differ is offered an episode only by its own name.
func TestGuessByName(t *testing.T) {
	cunk := []mediafiles.Item{ep(1, 1, "Beginnings"), ep(1, 2, "Episode 2"), ep(2, 1, "In the Beginnings"), ep(2, 2, "Faith/Off"), ep(2, 3, "Faith/Off")}
	for file, want := range map[string][2]int{
		"Cunk on Earth - S01E01 - In the Beginnings WEBDL-1080p.mkv": {2, 1}, // the longer name holds the shorter
		"Cunk on Britain - S01E01 - Beginnings.mkv":                  {1, 1},
		"Cunk on Earth - S01E02 - Faith Off.mkv":                     {}, // two episodes of that name
		"Cunk on Earth - S01E02 - Episode 2.mkv":                     {}, // a name that only counts
		"Cunk.on.Earth.S01E03.1080p.mkv":                             {}, // no name at all
	} {
		got := guessByName(file, cunk, "Cunk on...", "Cunk on Earth")
		switch {
		case want == [2]int{} && got != nil:
			t.Errorf("%s: guessed %+v", file, got)
		case want != [2]int{} && (got == nil || got.Season != want[0] || len(got.Episodes) != 1 || got.Episodes[0] != want[1]):
			t.Errorf("%s: %+v, want %v", file, got, want)
		}
	}
}

// The show's own name and release words are no episode name, words count
// whole, and a special is never offered.
func TestGuessByNameWholeWords(t *testing.T) {
	eps := []mediafiles.Item{ep(0, 1, "Cunk on Earth"), ep(1, 1, "Home"), ep(1, 2, "Earth"), ep(1, 3, "Pilot Light")}
	for file, want := range map[string][2]int{
		"Cunk on Earth - S01E01 - In the Beginnings WEBDL-1080p.mkv": {}, // the folder's name, and a special
		"Homeland - S01E01 - Pilot.mkv":                              {}, // "Home" inside a word, "Pilot" short of "Pilot Light"
		"Cunk on Earth - S01E02 - Home WEBDL-1080p.mkv":              {1, 1},
	} {
		got := guessByName(file, eps, "Cunk on...", "Cunk on Earth")
		switch {
		case want == [2]int{} && got != nil:
			t.Errorf("%s: guessed %+v", file, got)
		case want != [2]int{} && (got == nil || got.Season != want[0] || got.Episodes[0] != want[1]):
			t.Errorf("%s: %+v, want %v", file, got, want)
		}
	}
}
