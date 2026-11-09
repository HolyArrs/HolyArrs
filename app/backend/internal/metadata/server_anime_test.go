package metadata

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const animeBody = `{"entries":[
 {"anidb":100,"anilist":11,"mal":21,"tvdb":5000,"tvdb_season":1,"tmdb_tv":9000,"tmdb_season":1},
 {"anidb":101,"anilist":12,"mal":22,"tvdb":5000,"tvdb_season":1,"tvdb_offset":12,"tmdb_tv":9000,"tmdb_season":2},
 {"anidb":102,"anilist":13,"tmdb_tv":9000,"tmdb_season":2,"tmdb_offset":12},
 {"anidb":103,"anilist":14,"tmdb_tv":9000,"tmdb_season":0}
],"titles":[
 {"anilist":11,"romaji":"Gousei Shou","english":"Synthetic Show","native":"合成ショー"},
 {"anilist":12,"romaji":"Gousei Shou 2nd Season","english":"Synthetic Show Season 2"}
],"updated_at":"2026-10-21T00:00:00Z"}`

func TestAnimeMapReadsServer(t *testing.T) {
	var asked []string
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.RawQuery)
		if r.URL.Path != "/v1/anime/map" {
			w.WriteHeader(404)
			return
		}
		switch r.URL.RawQuery {
		case "tmdb=9000":
			_, _ = w.Write([]byte(animeBody))
		case "kind=movie&tmdb=7":
			_, _ = w.Write([]byte(`not json`))
		default:
			w.WriteHeader(404)
		}
	})
	m, err := s.AnimeMap(context.Background(), "tmdb_tv", "9000")
	if err != nil || len(m.Entries) != 4 || len(m.Titles) != 2 {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := s.AnimeMap(context.Background(), "mal", "1"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.AnimeMap(context.Background(), "tmdb_movie", "7"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatal(err)
	}
	for _, bad := range [][2]string{{"anidb", "1"}, {"tmdb_tv", "x"}, {"tmdb_tv", "0"}} {
		if _, err := s.AnimeMap(context.Background(), bad[0], bad[1]); !errors.Is(err, ErrServiceInvalid) {
			t.Fatalf("%v %v", bad, err)
		}
	}
	if strings.Join(asked, ",") != "tmdb=9000,mal=1,kind=movie&tmdb=7" {
		t.Fatalf("asked %v", asked)
	}
}

func TestAnimeSeasonsAndNames(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(animeBody)) })
	m, err := s.AnimeMap(context.Background(), "tmdb_tv", "9000")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.AltNames("synthetic show"), "|"); got != "Gousei Shou|合成ショー|Gousei Shou 2nd Season|Synthetic Show Season 2" {
		t.Fatalf("alt names %q", got)
	}
	ss := m.Seasons(map[int]int{0: 2, 1: 12, 2: 24})
	if len(ss) != 3 || ss[0].Number != 0 || ss[0].AbsoluteFrom != 0 {
		t.Fatalf("seasons %+v", ss)
	}
	s1, s2 := ss[1], ss[2]
	if s1.AniList != 11 || s1.MAL != 21 || s1.AbsoluteFrom != 1 || s1.Title == nil || s1.Title.Romaji != "Gousei Shou" || s1.Parts != nil {
		t.Fatalf("season 1 %+v", s1)
	}
	// Season 2 holds two cours: the second starts after TMDB episode 12.
	if s2.AniList != 12 || s2.AbsoluteFrom != 13 || len(s2.Parts) != 2 || s2.Parts[1].AniList != 13 || s2.Parts[1].Offset != 12 || s2.Parts[1].Title != nil {
		t.Fatalf("season 2 %+v", s2)
	}
}

func TestLooksAnime(t *testing.T) {
	for _, c := range []struct {
		genres  []string
		country []string
		lang    string
		want    bool
	}{
		{[]string{"Animation", "Drama"}, []string{"JP"}, "ja", true},
		{[]string{"animation"}, nil, "ja", true},
		{[]string{"Animation"}, []string{"US"}, "ja", false},
		{[]string{"Drama"}, []string{"JP"}, "ja", false},
		{[]string{"Animation"}, nil, "en", false},
	} {
		if got := LooksAnime(c.genres, c.country, c.lang); got != c.want {
			t.Fatalf("%+v: %v", c, got)
		}
	}
}

// Season numbers come from the server: absurd ones are dropped and the
// absolute count walks only the seasons that exist.
func TestAnimeSeasonsBounded(t *testing.T) {
	huge, neg, one, three := 2_000_000_000, -1, 1, 3
	m := AnimeMap{Entries: []AnimeEntry{{AniList: 1, TMDBSeason: &one}, {AniList: 2, TMDBSeason: &three}, {AniList: 3, TMDBSeason: &huge}, {AniList: 4, TMDBSeason: &neg}}}
	ss := m.Seasons(map[int]int{1: 10, 3: 5, huge: 9, -5: 4})
	if len(ss) != 2 || ss[0].Number != 1 || ss[0].AbsoluteFrom != 1 || ss[1].Number != 3 || ss[1].AbsoluteFrom != 11 {
		t.Fatalf("%+v", ss)
	}
}
