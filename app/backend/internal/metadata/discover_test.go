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
				"credits":{"cast":[{"id":21,"name":"Ann","character":"Hero","profile_path":"/a.jpg"}],"crew":[{"id":31,"name":"Dee","job":"Director"}]},
				"keywords":{"keywords":[{"name":"heist"},{"name":"bank robbery"},{"name":"double cross"},{"name":"time loop"}]},
				"images":{"backdrops":[{"file_path":"/b.jpg","vote_average":9},{"file_path":"/x1.jpg","vote_average":2},{"file_path":"/x2.jpg","vote_average":5},{"file_path":"/x3.jpg","vote_average":4},{"file_path":"/x4.jpg","vote_average":3},{"file_path":"/x5.jpg","vote_average":1}]},
				"videos":{"results":[{"key":"abcDEF12345","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en","name":"Official Trailer"},
					{"key":"vim123456","site":"Vimeo","type":"Trailer"},{"key":"clip1234567","site":"YouTube","type":"Clip"},{"key":"bad key!","site":"YouTube","type":"Teaser"}]},
				"belongs_to_collection":{"id":70,"name":"Seven Set"},
				"recommendations":` + list(15, "title") + `}`))
		case "/3/movie/7/watch/providers":
			_, _ = w.Write([]byte(`{"results":{"US":{"flatrate":[{"provider_id":8,"provider_name":"Netflix","logo_path":"/n.jpg","display_priority":2},{"provider_id":9,"provider_name":"Prime","display_priority":1}]},"GB":{"rent":[{"provider_id":2,"provider_name":"Apple"}]}}}`))
		case "/3/collection/70":
			_, _ = w.Write([]byte(`{"name":"The Seven Collection","parts":[{"id":72,"title":"Seven II","release_date":"2023-01-01","poster_path":"/s.jpg"},{"id":7,"title":"Seven","release_date":"2020-02-02","poster_path":"/s.jpg"}]}`))
		case "/3/person/31/movie_credits":
			_, _ = w.Write([]byte(`{"cast":[{"id":50,"title":"Acted","popularity":99}],"crew":[{"id":7,"title":"Seven","job":"Director","popularity":5},
				{"id":41,"title":"Low","job":"Director","popularity":1,"genre_ids":[18]},{"id":42,"title":"High","job":"Director","popularity":9},
				{"id":42,"title":"High","job":"Writer","popularity":9},{"id":43,"title":"Wrote","job":"Writer","popularity":50}]}`))
		case "/3/person/32/tv_credits":
			_, _ = w.Write([]byte(`{"crew":[{"id":81,"name":"Made","job":"Creator","popularity":3},{"id":82,"name":"Sound","job":"Sound","popularity":30}]}`))
		case "/3/tv/8/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"episode_number":1,"name":"Pilot","overview":"It starts.","air_date":"2021-01-01","runtime":50,"still_path":"/e1.jpg"},{"episode_number":2,"name":"Two","air_date":null}]}`))
		case "/3/tv/8":
			_, _ = w.Write([]byte(`{"id":8,"name":"Eight","first_air_date":"2021-01-01","created_by":[{"id":32,"name":"Cee"}],"keywords":{"results":[{"name":"small town"}]},
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
		d.People[0] != (DetailPerson{Name: "Ann", Role: "actor", Character: "Hero", Profile: ImageBase + "/w185/a.jpg", ID: "21"}) || d.People[1].Role != "director" {
		t.Fatalf("movie %+v", d)
	}
	if want := []string{ImageBase + "/w1280/x2.jpg", ImageBase + "/w1280/x3.jpg", ImageBase + "/w1280/x4.jpg", ImageBase + "/w1280/x1.jpg"}; fmt.Sprint(d.Backdrops) != fmt.Sprint(want) {
		t.Fatalf("backdrops %v", d.Backdrops)
	}
	if len(d.Videos) != 1 || d.Videos[0] != (DetailVideo{Key: "abcDEF12345", Name: "Official Trailer", Type: "Trailer", Official: true, Language: "en"}) {
		t.Fatalf("videos %+v", d.Videos)
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

func TestTMDBDetailsMoodsAndMadeBy(t *testing.T) {
	p := newTMDB(fakeDiscoverTMDB(t), fakeCred)
	ctx := context.Background()
	d, err := p.Details(ctx, "movie", "7")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(d.Moods) != "[Drama]" || d.People[1].ID != "31" || d.People[0].ID != "21" {
		t.Fatalf("moods %v people %+v", d.Moods, d.People)
	}
	s, err := p.Details(ctx, "show", "8")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(s.Moods) != "[]" || s.People[0].ID != "32" {
		t.Fatalf("show moods %v people %+v", s.Moods, s.People)
	}
	var _ MadeByLister = p
	m, err := p.MadeBy(ctx, "movie", "31")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 3 || m[0].Name != "High" || m[1].Name != "Seven" || m[2].Name != "Low" || fmt.Sprint(m[2].Genres) != "[Drama]" {
		t.Fatalf("made by %+v", m)
	}
	sh, err := p.MadeBy(ctx, "show", "32")
	if err != nil || len(sh) != 1 || sh[0].Name != "Made" || sh[0].Kind != "show" {
		t.Fatalf("shows %+v %v", sh, err)
	}
	for _, c := range [][2]string{{"movie", "0"}, {"movie", "x"}, {"artist", "31"}} {
		if _, err := p.MadeBy(ctx, c[0], c[1]); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%v: %v", c, err)
		}
	}
}

func TestMoods(t *testing.T) {
	cases := []struct {
		kw, genres []string
		want       string
	}{
		{[]string{"haunted house", "ghost", "demonic possession"}, []string{"Horror"}, "[Scary Horror]"},
		{[]string{"satire", "small town", "christmas"}, []string{"Comedy"}, "[Cozy Witty]"},
		{[]string{"award", "embrace", "glove"}, []string{"Drama", "Comedy", "Crime", "War"}, "[Drama Comedy Crime]"},
		{nil, nil, "[]"},
		{[]string{"murder", "conspiracy", "heist", "time travel", "romance", "zombie"}, []string{"Thriller"}, "[Suspenseful Thriller]"},
		{[]string{"wrestling", "sports entertainment", "underdog"}, []string{"Reality", "Talk"}, "[Sports Reality Talk]"},
		{[]string{"biography", "based on true story"}, []string{"Drama"}, "[Inspiring Drama]"},
	}
	for _, c := range cases {
		if got := fmt.Sprint(Moods(c.kw, c.genres)); got != c.want {
			t.Errorf("Moods(%v, %v) = %s, want %s", c.kw, c.genres, got, c.want)
		}
	}
}

func TestTMDBDetailsCollectionAndStreaming(t *testing.T) {
	p := newTMDB(fakeDiscoverTMDB(t), fakeCred)
	d, err := p.Details(context.Background(), "movie", "7")
	if err != nil {
		t.Fatal(err)
	}
	if d.Collection == nil || d.Collection.Name != "The Seven Collection" || len(d.Collection.Parts) != 2 || d.Collection.Parts[0].TMDBID != "7" || d.Collection.Parts[1].Name != "Seven II" {
		t.Fatalf("collection %+v", d.Collection)
	}
	us := d.Streaming["US"]
	if len(us) != 2 || us[0].Name != "Prime" || us[1].Logo != ImageBase+"/w92/n.jpg" || d.Streaming["GB"] != nil {
		t.Fatalf("streaming %+v", d.Streaming)
	}
}

func TestTMDBSeasonEpisodes(t *testing.T) {
	p := newTMDB(fakeDiscoverTMDB(t), fakeCred)
	var _ SeasonLister = p
	eps, err := p.SeasonEpisodes(context.Background(), "8", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 || eps[0] != (DetailEpisode{Number: 1, Name: "Pilot", Overview: "It starts.", Runtime: 50, Still: ImageBase + "/w780/e1.jpg", AirDay: "2021-01-01"}) || eps[1].AirDay != "" {
		t.Fatalf("episodes %+v", eps)
	}
	if _, err := p.SeasonEpisodes(context.Background(), "8", 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing season %v", err)
	}
	if _, err := p.SeasonEpisodes(context.Background(), "x", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id %v", err)
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
