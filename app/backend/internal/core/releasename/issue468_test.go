package releasename

import (
	"slices"
	"testing"
)

// Toloka names list audio languages as "Ukr/Eng" or "2xUkr/Eng",
// then subtitles after "Sub"; subtitle languages are not audio.
func TestTolokaLanguages(t *testing.T) {
	for name, want := range map[string][]string{
		"Oppenheimer (2023) BDRip-AVC Ukr/Eng | Sub Eng":                      {"uk", "en"},
		"The Bear S01 (2022) WEB-DL 1080p 2xUkr/Eng | Sub Eng":                {"uk", "en"},
		"Dune (1984) BDRip 1080p H.265 Ukr/Eng | Sub Eng [Theatrical Cut]":    {"uk", "en"},
		"Dune: Part One (2021) BDRemux 1080p 2xUkr/Eng | Sub Ukr/Eng":         {"uk", "en"},
		"Shogun S01 (2024) WEB-DL 4K 2160p H.265 HDR 3xUkr/Eng | Sub Ukr/Eng": {"uk", "en"},
		"Severance S02 (2025) WEB-DL-AVC Ukr | Sub Ukr/Eng":                   {"uk"},
		"Severance S01 (2022) WEB-DL 1080p Eng | Sub Ukr/Eng":                 {"en"},
	} {
		if got := Parse(name, Hints{}).Languages; !slices.Equal(got, want) {
			t.Errorf("%q: languages %v, want %v", name, got, want)
		}
	}
}

func TestTolokaLanguagesNoSpaces(t *testing.T) {
	if got := Parse("The Bear S01 (2022) WEB-DL 1080p 2xUkr/Eng|Sub Eng", Hints{}).Languages; !slices.Equal(got, []string{"uk", "en"}) {
		t.Errorf("languages %v, want [uk en]", got)
	}
}
