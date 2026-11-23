package sources

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Like the older movie app, a search adds the title's names in the
// Version's wanted languages and the naming title language, so a French or
// German copy is found under its French or German name, for movies and shows.
func TestWantedNames(t *testing.T) {
	old := mediafiles.TitleNamer
	t.Cleanup(func() { mediafiles.TitleNamer = old })
	names := map[string]string{"fr": "Le Voyage de Chihiro", "de": "Chihiros Reise ins Zauberland", "pt-br": "A Viagem de Chihiro",
		"es": "El viaje de Chihiro", "ja": "千と千尋の神隠し", "ko": "센과 치히로의 행방불명", "ar": "المخطوفة", "hi": "स्पिरिटेड अवे",
		"th": "มิติวิญญาณมหัศจรรย์", "zh-cn": "千与千寻", "ru": "Унесённые призраками", "en": "Spirited Away"}
	mediafiles.TitleNamer = func(kind, tmdbID, lang string) string {
		if tmdbID != "129" {
			return ""
		}
		return names[lang]
	}
	for _, kind := range []string{"movie", "show"} {
		ti := mediafiles.Title{Kind: kind, Name: "Spirited Away", SourceIDs: map[string]string{"tmdb": "129"}}
		cases := []struct {
			langs     []string
			titleLang string
			want      []string
		}{
			{[]string{"fr"}, "", []string{"Le Voyage de Chihiro"}},
			{[]string{"en", "de"}, "fr", []string{"Chihiros Reise ins Zauberland", "Le Voyage de Chihiro"}},
			{[]string{"pt"}, "", []string{"A Viagem de Chihiro"}},
			{[]string{"zh"}, "", []string{"千与千寻"}},
			{nil, "es", []string{"El viaje de Chihiro"}},
			{[]string{"ja", "ko", "ar", "hi", "th", "ru"}, "", []string{"千と千尋の神隠し", "센과 치히로의 행방불명", "المخطوفة", "स्पिरिटेड अवे", "มิติวิญญาณมหัศจรรย์", "Унесённые призраками"}},
			{[]string{"en"}, "en", nil},
			{[]string{"fr"}, "fr", []string{"Le Voyage de Chihiro"}},
		}
		for _, tc := range cases {
			if got := wantedNames(ti, tc.langs, tc.titleLang); !slices.Equal(got, tc.want) {
				t.Errorf("%s %v %q: %q, want %q", kind, tc.langs, tc.titleLang, got, tc.want)
			}
		}
	}
	// No TMDB id: no kept names.
	if got := wantedNames(mediafiles.Title{Kind: "movie", Name: "X"}, []string{"fr"}, ""); got != nil {
		t.Errorf("no id: %q", got)
	}
}

// The wanted names go first and are asked even for an English title, whose
// other names are otherwise never asked.
func TestWithWanted(t *testing.T) {
	q := Query{Title: "Spirited Away", Aliases: []string{"Sen to Chihiro no Kamikakushi"}}
	q.other.n = 0
	withWanted(&q, []string{"Le Voyage de Chihiro"})
	if q.other.n != 1 || q.other.wanted != 1 || q.Aliases[0] != "Le Voyage de Chihiro" || len(q.Aliases) != 2 {
		t.Fatalf("got n %d wanted %d aliases %q", q.other.n, q.other.wanted, q.Aliases)
	}
	q = Query{Title: "Spirited Away"}
	q.other.n = maxNameSearches
	withWanted(&q, []string{"a", "b", "c", "d"})
	if q.other.n != maxNameSearches || q.other.wanted != maxNameSearches {
		t.Fatalf("cap: n %d wanted %d", q.other.n, q.other.wanted)
	}
	q = Query{Title: "Spirited Away", Aliases: []string{"x"}}
	withWanted(&q, nil)
	if q.other.n != 0 || len(q.Aliases) != 1 {
		t.Fatalf("none: %+v", q.other)
	}
}
