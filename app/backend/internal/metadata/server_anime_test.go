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

// A TMDB season that holds several cours gets the TVDB-style numbers the
// releases use; specials, absolute entries and other shows are left alone.
func TestAnimeReleaseNumbers(t *testing.T) {
	n := func(v int) *int { return &v }
	entry := func(tmdbID int64, tmdbSeason, tmdbOff, tvdbSeason, tvdbOff int) AnimeEntry {
		return AnimeEntry{TMDBTV: tmdbID, TMDBSeason: n(tmdbSeason), TMDBOffset: tmdbOff, TVDBSeason: n(tvdbSeason), TVDBOffset: tvdbOff}
	}
	eps := func(season, count int) []Episode {
		out := []Episode{}
		for i := 1; i <= count; i++ {
			out = append(out, Episode{Season: season, Number: i})
		}
		return out
	}
	alt := func(eps []Episode, season, number int) [2]int {
		for _, e := range eps {
			if e.Season == season && e.Number == number {
				return [2]int{e.AltSeason, e.AltEpisode}
			}
		}
		t.Fatalf("no S%02dE%02d", season, number)
		return [2]int{}
	}
	mashle := []AnimeEntry{entry(204832, 1, 0, 1, 0), entry(204832, 1, 12, 2, 0)}

	got := append(eps(0, 1), eps(1, 25)...)
	AnimeMap{Entries: mashle}.SetReleaseNumbers("204832", []Season{{Number: 0}, {Number: 1}}, got)
	for _, c := range []struct{ season, number, altS, altE int }{
		{1, 5, 0, 0}, {1, 12, 0, 0}, {1, 13, 2, 1}, {1, 17, 2, 5}, {1, 25, 2, 13}, {0, 1, 0, 0},
	} {
		if a := alt(got, c.season, c.number); a != [2]int{c.altS, c.altE} {
			t.Errorf("S%02dE%02d: alt %v, want %d/%d", c.season, c.number, a, c.altS, c.altE)
		}
	}

	three := eps(1, 36)
	AnimeMap{Entries: []AnimeEntry{entry(5, 1, 0, 1, 0), entry(5, 1, 12, 2, 0), entry(5, 1, 24, 3, 0)}}.SetReleaseNumbers("5", []Season{{Number: 1}}, three)
	if a := alt(three, 1, 30); a != [2]int{3, 6} {
		t.Errorf("three cours: S01E30 alt %v", a)
	}

	for name, entries := range map[string][]AnimeEntry{
		"one entry per season": {entry(5, 1, 0, 1, 0), entry(5, 2, 0, 2, 0)},
		"absolute entry":       {entry(5, 1, 0, 1, 0), {TMDBTV: 5, TMDBSeason: n(1), TMDBOffset: 12, TVDBSeason: n(2), Absolute: true}},
		"tvdb season 0":        {entry(5, 1, 0, 1, 0), entry(5, 1, 12, 0, 0)},
		"other show":           {entry(5, 1, 0, 1, 0), entry(6, 1, 12, 2, 0)},
	} {
		list := eps(1, 24)
		AnimeMap{Entries: entries}.SetReleaseNumbers("5", []Season{{Number: 1}, {Number: 2}}, list)
		for _, e := range list {
			if e.AltSeason != 0 || e.AltEpisode != 0 {
				t.Errorf("%s: S%02dE%02d got alt %d/%d", name, e.Season, e.Number, e.AltSeason, e.AltEpisode)
			}
		}
	}
}

// A map that names TMDB seasons the show no longer has is out of date, so
// it numbers nothing, even inside the seasons TMDB still has.
func TestAnimeReleaseNumbersStaleSeasons(t *testing.T) {
	n := func(v int) *int { return &v }
	entry := func(tmdbSeason, tmdbOff, tvdbSeason int) AnimeEntry {
		return AnimeEntry{TMDBTV: 5, TMDBSeason: n(tmdbSeason), TMDBOffset: tmdbOff, TVDBSeason: n(tvdbSeason)}
	}
	list := []Episode{}
	for i := 1; i <= 36; i++ {
		list = append(list, Episode{Season: 1, Number: i})
	}
	stale := AnimeMap{Entries: []AnimeEntry{entry(1, 0, 1), entry(1, 12, 2), entry(2, 0, 3)}}
	if stale.SetReleaseNumbers("5", []Season{{Number: 1}}, list) {
		t.Error("stale map reported as used")
	}
	for _, e := range list {
		if e.AltSeason != 0 || e.AltEpisode != 0 {
			t.Fatalf("S%02dE%02d got alt %d/%d", e.Season, e.Number, e.AltSeason, e.AltEpisode)
		}
	}

	// A season TMDB lists but has no episodes in yet (an announced next
	// part) still exists, so the map stays good.
	list = list[:24]
	if !stale.SetReleaseNumbers("5", []Season{{Number: 0}, {Number: 1}, {Number: 2}}, list) {
		t.Error("map with an empty listed season reported as unused")
	}
	if e := list[16]; e.AltSeason != 2 || e.AltEpisode != 5 {
		t.Errorf("empty listed season: S01E17 alt %d/%d, want 2/5", e.AltSeason, e.AltEpisode)
	}
}

// A map that knows the show reports itself used, even when its layout
// agrees with TMDB's and it numbers nothing; one that does not know it
// does not.
func TestAnimeReleaseNumbersUsed(t *testing.T) {
	n := func(v int) *int { return &v }
	list := []Episode{}
	for i := 1; i <= 24; i++ {
		list = append(list, Episode{Season: 1, Number: i})
	}
	seasons := []Season{{Number: 1}}
	agree := AnimeMap{Entries: []AnimeEntry{
		{TMDBTV: 5, TMDBSeason: n(1), TVDBSeason: n(1)},
		{TMDBTV: 5, TMDBSeason: n(1), TMDBOffset: 12, TVDBSeason: n(1), TVDBOffset: 12},
	}}
	if !agree.SetReleaseNumbers("5", seasons, list) || list[16].AltSeason != 0 {
		t.Errorf("agreeing map: alt %d/%d", list[16].AltSeason, list[16].AltEpisode)
	}
	if !(AnimeMap{Entries: []AnimeEntry{{TMDBTV: 5, TMDBSeason: n(1), TVDBSeason: n(1)}}}).SetReleaseNumbers("5", seasons, list) {
		t.Error("one part per season reported as unused")
	}
	if (AnimeMap{Entries: []AnimeEntry{{TMDBTV: 6, TMDBSeason: n(1), TVDBSeason: n(1)}}}).SetReleaseNumbers("5", seasons, list) {
		t.Error("other show's map reported as used")
	}
}

// TVMaze's season sizes split one long TMDB season the way release names
// do; layouts that agree or do not fit are left alone.
func TestSeasonSizeReleaseNumbers(t *testing.T) {
	eps := func(counts ...int) []Episode {
		out := []Episode{{Season: 0, Number: 1, AirDay: "2023-01-01"}}
		for s, c := range counts {
			for i := 1; i <= c; i++ {
				out = append(out, Episode{Season: s + 1, Number: i, AirDay: "2023-04-01"})
			}
		}
		return out
	}
	alt := func(list []Episode, season, number int) [2]int {
		for _, e := range list {
			if e.Season == season && e.Number == number {
				return [2]int{e.AltSeason, e.AltEpisode}
			}
		}
		t.Fatalf("no S%02dE%02d", season, number)
		return [2]int{}
	}

	mashle := eps(25)
	mashle[len(mashle)-1].AirDay = ""
	setSeasonSizes(mashle, []int{12, 12})
	for _, c := range []struct{ season, number, altS, altE int }{
		{1, 5, 0, 0}, {1, 12, 0, 0}, {1, 13, 2, 1}, {1, 17, 2, 5}, {1, 24, 2, 12}, {1, 25, 2, 13}, {0, 1, 0, 0},
	} {
		if a := alt(mashle, c.season, c.number); a != [2]int{c.altS, c.altE} {
			t.Errorf("S%02dE%02d: alt %v, want %d/%d", c.season, c.number, a, c.altS, c.altE)
		}
	}

	two := eps(12, 24)
	setSeasonSizes(two, []int{12, 12, 12})
	if a, b := alt(two, 2, 1), alt(two, 2, 13); a != [2]int{} || b != [2]int{3, 1} {
		t.Errorf("two TMDB seasons: S02E01 %v, S02E13 %v", a, b)
	}

	for name, c := range map[string]struct {
		counts []int
		sizes  []int
	}{
		"layouts agree":     {[]int{24}, []int{24}},
		"too few episodes":  {[]int{24}, []int{12, 6}},
		"split misaligned":  {[]int{10, 14}, []int{12, 6, 6}},
		"empty last season": {[]int{24}, []int{12, 12, 12}},
		"no sizes":          {[]int{24}, nil},
	} {
		list := eps(c.counts...)
		setSeasonSizes(list, c.sizes)
		for _, e := range list {
			if e.AltSeason != 0 || e.AltEpisode != 0 {
				t.Errorf("%s: S%02dE%02d got alt %d/%d", name, e.Season, e.Number, e.AltSeason, e.AltEpisode)
			}
		}
	}
}
