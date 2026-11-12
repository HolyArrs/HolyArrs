package metadata

import (
	"encoding/json"
	"slices"
	"testing"
)

// A name search answer has genre_ids, not genre names: the hides and the
// plain-words search still need the names.
func TestSearchGenreIDsBecomeNames(t *testing.T) {
	var m tmdbMovie
	if err := json.Unmarshal([]byte(`{"id": 7, "title": "Dark House", "genre_ids": [27, 53, 999999]}`), &m); err != nil {
		t.Fatal(err)
	}
	if got := m.movie("https://img").Genres; !slices.Equal(got, []string{"Horror", "Thriller"}) {
		t.Fatalf("movie genres = %v", got)
	}
	var s tmdbShow
	if err := json.Unmarshal([]byte(`{"id": 8, "name": "Far Stars", "genre_ids": [10765, 18]}`), &s); err != nil {
		t.Fatal(err)
	}
	if got := s.show("https://img").Genres; !slices.Equal(got, []string{"Sci-Fi & Fantasy", "Drama"}) {
		t.Fatalf("show genres = %v", got)
	}
	// Named genres (a details answer) win over the ids.
	if err := json.Unmarshal([]byte(`{"id": 7, "title": "Dark House", "genres": [{"name": "Comedy"}], "genre_ids": [27]}`), &m); err != nil {
		t.Fatal(err)
	}
	if got := m.movie("https://img").Genres; !slices.Equal(got, []string{"Comedy"}) {
		t.Fatalf("named genres = %v", got)
	}
	c := tmdbCredit{ID: 9, Title: "Old Fear", GenreIDs: []int64{27}}
	if got := c.movie("https://img").Genres; !slices.Equal(got, []string{"Horror"}) {
		t.Fatalf("credit genres = %v", got)
	}
}
