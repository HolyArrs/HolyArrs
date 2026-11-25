package releasename

import (
	"slices"
	"testing"
)

// A Source's no-language list fills in a name that shows no language
// (also a bare MULTi or DUAL); a name that names one keeps its own.
func TestSourceNoLanguage(t *testing.T) {
	uk := []string{"uk"}
	for _, c := range []struct {
		name  string
		none  []string
		multi []string
		want  []string
	}{
		{"The Odyssey 2026 WEBRip 1080p-SPiLNO", uk, nil, []string{"uk"}},
		{"The Odyssey 2026 WEBRip 1080p-SPiLNO", nil, nil, nil},
		{"Severance (Season 1) (2022) WEB-DL 1080p Eng", uk, nil, []string{"en"}},
		{"Movie.Title.2019.Ukr.Eng.1080p.WEB-DL-GRP", uk, nil, []string{"uk", "en"}},
		{"Movie.Title.2019.MULTi.1080p.BluRay.x264-GRP", uk, nil, []string{"uk"}},
		{"Movie.Title.2019.MULTi.1080p.BluRay.x264-GRP", uk, []string{"fr", "en"}, []string{"fr", "en"}},
		{"Movie.Title.2019.DUAL.1080p.BluRay.x264-GRP", []string{"it", "en"}, nil, []string{"it", "en"}},
	} {
		got := Parse(c.name, Hints{NoLanguage: c.none, MultiLanguages: c.multi}).Languages
		if !slices.Equal(got, c.want) {
			t.Errorf("%s with %v/%v: languages %v, want %v", c.name, c.none, c.multi, got, c.want)
		}
	}
}

// Taking title words out of a name keeps the languages its Source added.
func TestSourceNoLanguageSurvivesTitleWords(t *testing.T) {
	name := "Lost in Translation 2003 1080p BluRay x264-GRP"
	p := Parse(name, Hints{NoLanguage: []string{"uk"}})
	got := WithoutTitleWords(name, Hints{}, p.Languages, "Lost in Translation")
	if !slices.Equal(got, []string{"uk"}) {
		t.Fatalf("languages %v, want [uk]", got)
	}
}
