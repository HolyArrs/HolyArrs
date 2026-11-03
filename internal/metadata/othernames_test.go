package metadata

import (
	"encoding/json"
	"slices"
	"testing"
)

// TMDB's original, alternative and translated names all become OtherNames.
func TestTMDBOtherNames(t *testing.T) {
	var m tmdbMovie
	if err := json.Unmarshal([]byte(`{"id":194,"title":"Amelie","original_title":"Le Fabuleux Destin d'Amélie Poulain",
		"alternative_titles":{"titles":[{"title":"Amelie from Montmartre"}]},
		"translations":{"translations":[{"data":{"title":"Die fabelhafte Welt der Amelie"}}]}}`), &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"Le Fabuleux Destin d'Amélie Poulain", "Amelie from Montmartre", "Die fabelhafte Welt der Amelie"}
	if got := m.movie("").OtherNames; !slices.Equal(got, want) {
		t.Errorf("movie names %q", got)
	}
	var s tmdbShow
	if err := json.Unmarshal([]byte(`{"id":71446,"name":"Money Heist","original_name":"La casa de papel",
		"alternative_titles":{"results":[{"title":"Haus des Geldes"}]},
		"translations":{"translations":[{"data":{"name":"Casa de Papel"}}]}}`), &s); err != nil {
		t.Fatal(err)
	}
	want = []string{"La casa de papel", "Haus des Geldes", "Casa de Papel"}
	if got := s.show("").OtherNames; !slices.Equal(got, want) {
		t.Errorf("show names %q", got)
	}
}
