package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// FakeDiscoverTMDB answers the discover and details paths with fixed data.
func fakeDiscoverTMDB(t *testing.T) *httptest.Server {
	t.Helper()
	list := func(n int, title string) string {
		var parts []string
		for i := 1; i <= n; i++ {
			parts = append(parts, fmt.Sprintf(`{"id":%d,"%s":"T%d","release_date":"2024-01-01","first_air_date":"2023-01-01","poster_path":"/p%d.jpg","backdrop_path":"/b%d.jpg","genre_ids":[18,99]}`, i, title, i, i, i))
		}
		parts = append(parts, `{"id":999,"title":"Adult","adult":true}`)
		return `{"results":[` + strings.Join(parts, ",") + `]}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"},{"id":35,"name":"Comedy"}]}`))
		case "/3/trending/movie/week", "/3/movie/popular", "/3/movie/upcoming", "/3/movie/7/recommendations":
			_, _ = w.Write([]byte(list(25, "title")))
		case "/3/trending/tv/week", "/3/tv/on_the_air":
			_, _ = w.Write([]byte(list(3, "name")))
		case "/3/discover/movie":
			if r.URL.Query().Get("with_genres") != "35" {
				t.Errorf("genre query %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(list(2, "title")))
		case "/3/movie/7":
			_, _ = w.Write([]byte(`{"id":7,"title":"Seven","release_date":"2020-02-02","runtime":99,"backdrop_path":"/b.jpg","genres":[{"name":"Drama"}],
				"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"type":3,"certification":"R","release_date":"2020-02-02"}]}]},
				"credits":{"cast":[{"name":"Ann","character":"Hero","profile_path":"/a.jpg"}],"crew":[{"name":"Dee","job":"Director"}]},
				"recommendations":` + list(15, "title") + `}`))
		case "/3/tv/8":
			_, _ = w.Write([]byte(`{"id":8,"name":"Eight","first_air_date":"2021-01-01","created_by":[{"name":"Cee"}],
				"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-MA"}]},
				"seasons":[{"season_number":1,"episode_count":10,"air_date":"2021-01-01"}]}`))
		case "/3/movie/429/recommendations":
			w.WriteHeader(429)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTMDBDiscoverRows(t *testing.T) {
	p := newTMDB(fakeDiscoverTMDB(t), fakeCred)
	var _ Discoverer = p
	ctx := context.Background()
	cases := []struct {
		row, kind, ref, genre string
		n                     int
	}{
		{"trending", "movie", "", "", 25}, {"trending", "show", "", "", 3}, {"popular", "movie", "", "", 25},
		{"upcoming", "movie", "", "", 25}, {"upcoming", "show", "", "", 3}, {"similar", "movie", "7", "", 25},
		{"genre", "movie", "", "comedy", 2}, {"genre", "movie", "", "Western", 0},
	}
	for _, c := range cases {
		items, err := p.Discover(ctx, c.row, c.kind, c.ref, c.genre)
		if err != nil || len(items) != c.n {
			t.Fatalf("%s %s: %d items, %v", c.row, c.kind, len(items), err)
		}
		if c.n > 0 {
			it := items[0]
			if it.TMDBID != "1" || it.Kind != c.kind || it.Name != "T1" || it.Poster != ImageBase+"/w342/p1.jpg" ||
				it.Backdrop != ImageBase+"/w1280/b1.jpg" || len(it.Genres) != 1 || it.Genres[0] != "Drama" {
				t.Fatalf("%s item %+v", c.row, it)
			}
		}
	}
	if _, err := p.Discover(ctx, "similar", "movie", "429", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("rate limit error %v", err)
	}
}

func TestTMDBDetails(t *testing.T) {
	p := newTMDB(fakeDiscoverTMDB(t), fakeCred)
	ctx := context.Background()
	d, err := p.Details(ctx, "movie", "7")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Seven" || d.Year != 2020 || d.RuntimeMinutes != 99 || d.Rating != "R" || len(d.Similar) != 12 || len(d.People) != 2 ||
		d.People[0] != (DetailPerson{Name: "Ann", Role: "actor", Character: "Hero", Profile: ImageBase + "/w185/a.jpg"}) || d.People[1].Role != "director" {
		t.Fatalf("movie %+v", d)
	}
	s, err := p.Details(ctx, "show", "8")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Eight" || s.Rating != "TV-MA" || len(s.Seasons) != 1 || s.Seasons[0] != (DetailSeason{1, 10, 2021}) ||
		len(s.People) != 1 || s.People[0].Role != "creator" || s.Similar == nil {
		t.Fatalf("show %+v", s)
	}
	if _, err := p.Details(ctx, "movie", "404"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown %v", err)
	}
	if _, err := p.Details(ctx, "artist", "7"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("kind %v", err)
	}
}

func TestBestQuery(t *testing.T) {
	now := time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC)
	m := BestQuery("movie", now)
	if m.Get("primary_release_date.gte") != "2024-10-19" || m.Get("vote_average.gte") != "7.5" || m.Get("vote_count.gte") != "300" ||
		m.Get("sort_by") != "vote_average.desc" || m.Has("first_air_date.gte") {
		t.Fatalf("movie %v", m)
	}
	s := BestQuery("show", now)
	if s.Get("first_air_date.gte") != "2024-10-19" || s.Has("primary_release_date.gte") {
		t.Fatalf("show %v", s)
	}
}
