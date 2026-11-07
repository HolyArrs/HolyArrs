package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The fakes answer the TV endpoints of each source with one lab show:
// two regular seasons, a special and an episode with no air date yet.

func fakeTMDBTV(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeCred {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/search/tv":
			if r.URL.Query().Get("query") != "lab show" {
				t.Errorf("tv search %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"results":[{"id":100,"name":"Lab Show","first_air_date":"2025-01-05","overview":"A test show.","original_language":"en","poster_path":"/s.jpg"}]}`))
		case "/3/tv/100":
			if r.URL.Query().Get("append_to_response") != "external_ids,images,credits,content_ratings,translations,alternative_titles" {
				t.Errorf("tv details %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"id":100,"name":"Lab Show","first_air_date":"2025-01-05","overview":"A test show.","original_language":"en",
				"status":"Returning Series","networks":[{"name":"Lab TV"}],"episode_run_time":[42],"poster_path":"/s.jpg",
				"external_ids":{"imdb_id":"tt0100","tvdb_id":200},
				"seasons":[{"season_number":0,"name":"Specials","id":10},{"season_number":1,"name":"Season 1","id":11},{"season_number":2,"name":"Season 2","id":12}]}`))
		case "/3/tv/100/season/0":
			_, _ = w.Write([]byte(`{"episodes":[{"id":1000,"season_number":0,"episode_number":1,"name":"Behind the Lab","air_date":"2025-03-01"}]}`))
		case "/3/tv/100/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"id":1101,"season_number":1,"episode_number":1,"name":"Pilot","air_date":"2025-01-05","runtime":44},
				{"id":1102,"season_number":1,"episode_number":2,"name":"Second","air_date":"2025-01-12","runtime":42}]}`))
		case "/3/tv/100/season/2":
			_, _ = w.Write([]byte(`{"episodes":[{"id":1201,"season_number":2,"episode_number":1,"name":"Return","air_date":"2026-01-04"},
				{"id":1202,"season_number":2,"episode_number":2,"name":"","air_date":null}]}`))
		case "/3/find/200", "/3/find/tt0100":
			want := map[string]string{"/3/find/200": "tvdb_id", "/3/find/tt0100": "imdb_id"}[r.URL.Path]
			if r.URL.Query().Get("external_source") != want {
				t.Errorf("find %s %v", r.URL.Path, r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"tv_results":[{"id":100}],"movie_results":[]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeTVMaze(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("TVMaze needs no key; nothing may be sent")
		}
		w.Header().Set("Content-Type", "application/json")
		show := `{"id":300,"name":"Lab Show","premiered":"2025-01-05","summary":"<p>A <b>test</b> show.</p>","language":"English","status":"Running",
			"averageRuntime":43,"schedule":{"time":"21:00","days":["Sunday"]},"network":{"name":"Lab TV","country":{"timezone":"America/New_York"}},
			"externals":{"thetvdb":200,"imdb":"tt0100"},"image":{"original":"https://static.tvmaze.test/s.jpg"}}`
		switch {
		case r.URL.Path == "/search/shows":
			_, _ = w.Write([]byte(`[{"score":0.9,"show":` + show + `}]`))
		case r.URL.Path == "/shows/300":
			_, _ = w.Write([]byte(show))
		case r.URL.Path == "/lookup/shows":
			q := r.URL.Query()
			if q.Get("thetvdb") != "200" && q.Get("imdb") != "tt0100" {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write([]byte(show))
		case r.URL.Path == "/shows/300/episodes":
			if r.URL.Query().Get("specials") != "1" {
				t.Errorf("episodes %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`[
				{"id":3101,"season":1,"number":1,"name":"Pilot","airdate":"2025-01-05","airtime":"21:00","airstamp":"2025-01-06T02:00:00+00:00","runtime":44,"type":"regular"},
				{"id":3102,"season":1,"number":2,"name":"Second","airdate":"2025-01-12","airtime":"21:00","airstamp":"2025-01-13T02:00:00+00:00","runtime":42,"type":"regular"},
				{"id":3001,"season":1,"number":null,"name":"Behind the Lab","airdate":"2025-03-01","airtime":"","airstamp":"2025-03-01T12:00:00+00:00","type":"significant_special"},
				{"id":3201,"season":2,"number":1,"name":"Return","airdate":"2026-01-04","airtime":"21:00","airstamp":"2026-01-05T02:00:00+00:00","type":"regular"},
				{"id":3202,"season":2,"number":2,"name":"TBA","airdate":"","airtime":"","airstamp":null,"type":"regular"}]`))
		case r.URL.Path == "/shows/429":
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

const fakeTVDBKey = "test-tvdb-value"

// fakeSession is what the fake TVDB hands out after a login.
const fakeSession = "lab-session-value"

func fakeTVDB(t *testing.T, logins *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v4/login" {
			logins.Add(1)
			var body struct {
				APIKey string `json:"apikey"`
			}
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || body.APIKey != fakeTVDBKey {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"token":"` + fakeSession + `"}}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+fakeSession {
			w.WriteHeader(401)
			return
		}
		if strings.Contains(r.URL.RawQuery, fakeTVDBKey) {
			t.Error("the key must never be in a URL")
		}
		switch r.URL.Path {
		case "/v4/search":
			if q := r.URL.Query(); q.Get("query") != "lab show" || q.Get("type") != "series" {
				t.Errorf("tvdb search %v", q)
			}
			_, _ = w.Write([]byte(`{"data":[{"tvdb_id":"200","name":"Lab Show","year":"2025","overview":"A test show.","primary_language":"eng","image_url":"https://artworks.tvdb.test/s.jpg"}]}`))
		case "/v4/series/200/extended":
			_, _ = w.Write([]byte(`{"data":{"id":200,"name":"Lab Show","year":"2025","firstAired":"2025-01-05","overview":"A test show.","originalLanguage":"eng",
				"status":{"name":"Continuing"},"airsTime":"21:00","averageRuntime":43,"originalNetwork":{"name":"Lab TV"},"image":"https://artworks.tvdb.test/s.jpg",
				"remoteIds":[{"id":"tt0100","sourceName":"IMDB"},{"id":"100","sourceName":"TheMovieDB.com"},{"id":"300","sourceName":"TV Maze"}]}}`))
		case "/v4/series/200/episodes/default":
			_, _ = w.Write([]byte(`{"data":{"episodes":[
				{"id":2101,"seasonNumber":1,"number":1,"absoluteNumber":1,"name":"Pilot","aired":"2025-01-05","runtime":44},
				{"id":2102,"seasonNumber":1,"number":2,"absoluteNumber":2,"name":"Second","aired":"2025-01-12","runtime":42},
				{"id":2001,"seasonNumber":0,"number":1,"absoluteNumber":0,"name":"Behind the Lab","aired":"2025-03-01"},
				{"id":2201,"seasonNumber":2,"number":1,"absoluteNumber":3,"name":"Return","aired":"2026-01-04"},
				{"id":2202,"seasonNumber":2,"number":2,"absoluteNumber":4,"name":"TBA","aired":null}]},"links":{"next":null}}`))
		case "/v4/search/remoteid/tt0100", "/v4/search/remoteid/100":
			_, _ = w.Write([]byte(`{"data":[{"series":{"id":200}}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func ep(s Show, season, number int) *Episode {
	for i := range s.Episodes {
		if s.Episodes[i].Season == season && s.Episodes[i].Number == number {
			return &s.Episodes[i]
		}
	}
	return nil
}

func at(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

// checkLabShow checks what every source must agree on.
func checkLabShow(t *testing.T, src string, s Show) {
	t.Helper()
	if s.Source != src || s.Name != "Lab Show" || s.Year != 2025 || s.Status != "running" || s.Network != "Lab TV" || s.IDs["tvdb"] != "200" || s.IDs["imdb"] != "tt0100" {
		t.Fatalf("%s show %+v", src, s)
	}
	if len(s.Episodes) != 5 {
		t.Fatalf("%s episodes %+v", src, s.Episodes)
	}
	p := ep(s, 1, 1)
	if p == nil || p.Name != "Pilot" || p.AirDay != "2025-01-05" || p.AirDate == nil || p.ID == "" {
		t.Fatalf("%s pilot %+v", src, p)
	}
	if sp := ep(s, 0, 1); sp == nil || sp.Name != "Behind the Lab" || sp.AirDay != "2025-03-01" {
		t.Fatalf("%s special %+v", src, sp)
	}
	if tba := ep(s, 2, 2); tba == nil || tba.AirDate != nil || tba.AirDay != "" {
		t.Fatalf("%s no air date %+v", src, tba)
	}
	if len(s.Seasons) != 3 || s.Seasons[0].Number != 0 || s.Seasons[2].Number != 2 {
		t.Fatalf("%s seasons %+v", src, s.Seasons)
	}
}

func TestTMDBShows(t *testing.T) {
	p := newTMDB(fakeTMDBTV(t), fakeCred)
	ctx := context.Background()
	res, err := p.SearchShows(ctx, "lab show")
	if err != nil || len(res) != 1 || res[0].IDs["tmdb"] != "100" || res[0].Year != 2025 || !strings.HasSuffix(res[0].Poster, "/w500/s.jpg") {
		t.Fatalf("search %+v %v", res, err)
	}
	s, err := p.Show(ctx, "tmdb:100")
	if err != nil {
		t.Fatal(err)
	}
	checkLabShow(t, "tmdb", s)
	// TMDB knows the day only: the episode counts as aired once that day
	// has ended in UTC.
	if got := ep(s, 1, 1).AirDate; !got.Equal(at("2025-01-06T00:00:00Z")) {
		t.Fatalf("pilot air time %v", got)
	}
	if ep(s, 1, 1).Runtime != 44 || s.Runtime != 42 {
		t.Fatalf("runtimes %+v %d", ep(s, 1, 1), s.Runtime)
	}
	// Other ids are found through TMDB's find.
	for _, id := range []string{"tvdb:200", "imdb:tt0100"} {
		if s, err := p.Show(ctx, id); err != nil || s.IDs["tmdb"] != "100" {
			t.Fatalf("%s: %+v %v", id, s.IDs, err)
		}
	}
	if _, err := p.Show(ctx, "tvmaze:300"); !errors.Is(err, ErrShowNotFound) {
		t.Fatalf("tvmaze id on TMDB: %v", err)
	}
	if _, err := p.Show(ctx, "tmdb:999"); !errors.Is(err, ErrShowNotFound) {
		t.Fatalf("unknown show: %v", err)
	}
	if _, err := newTMDB(fakeTMDBTV(t), "wrong").Show(ctx, "tmdb:100"); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("wrong token: %v", err)
	}
}

func TestTVMazeShows(t *testing.T) {
	var hits atomic.Int32
	srv := fakeTVMaze(t, &hits)
	p := &TVMaze{BaseURL: srv.URL, Gap: time.Millisecond}
	ctx := context.Background()
	res, err := p.SearchShows(ctx, "lab show")
	if err != nil || len(res) != 1 || res[0].IDs["tvmaze"] != "300" || res[0].Overview != "A test show." {
		t.Fatalf("search %+v %v", res, err)
	}
	s, err := p.Show(ctx, "tvmaze:300")
	if err != nil {
		t.Fatal(err)
	}
	checkLabShow(t, "tvmaze", s)
	if s.AirTime != "21:00" || s.TimeZone != "America/New_York" || s.OriginalLanguage != "en" {
		t.Fatalf("schedule %+v", s)
	}
	// TVMaze gives the exact air time.
	if got := ep(s, 1, 1).AirDate; !got.Equal(at("2025-01-06T02:00:00Z")) {
		t.Fatalf("pilot air time %v", got)
	}
	// A TVDB or IMDb id is looked up.
	for _, id := range []string{"tvdb:200", "imdb:tt0100"} {
		if s, err := p.Show(ctx, id); err != nil || s.IDs["tvmaze"] != "300" || len(s.Episodes) != 5 {
			t.Fatalf("%s: %+v %v", id, s.IDs, err)
		}
	}
	if _, err := p.Show(ctx, "tvdb:999"); !errors.Is(err, ErrShowNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := p.Show(ctx, "tmdb:100"); !errors.Is(err, ErrShowNotFound) {
		t.Fatalf("tmdb id on TVMaze: %v", err)
	}
	// Results are cached: asking again sends nothing, unless the cache is skipped.
	n := hits.Load()
	_, _ = p.Show(ctx, "tvmaze:300")
	if hits.Load() != n {
		t.Fatal("cached show fetched again")
	}
	_, _ = p.Show(NoCache(ctx), "tvmaze:300")
	if hits.Load() == n {
		t.Fatal("NoCache was not honoured")
	}
	if _, err := p.Show(ctx, "tvmaze:429"); err == nil {
		t.Fatal("rate limit twice must fail")
	}
}

func TestTVDBShows(t *testing.T) {
	var logins atomic.Int32
	srv := fakeTVDB(t, &logins)
	key := fakeTVDBKey
	p := &TVDB{BaseURL: srv.URL + "/v4", Key: func(context.Context) (string, error) { return key, nil }, Gap: time.Millisecond}
	ctx := context.Background()
	res, err := p.SearchShows(ctx, "lab show")
	if err != nil || len(res) != 1 || res[0].IDs["tvdb"] != "200" || res[0].Year != 2025 {
		t.Fatalf("search %+v %v", res, err)
	}
	s, err := p.Show(ctx, "tvdb:200")
	if err != nil {
		t.Fatal(err)
	}
	checkLabShow(t, "tvdb", s)
	if ep(s, 2, 1).Absolute != 3 || ep(s, 0, 1).Absolute != 0 || s.IDs["tmdb"] != "100" || s.IDs["tvmaze"] != "300" || s.AirTime != "21:00" {
		t.Fatalf("tvdb details %+v %+v", s, ep(s, 2, 1))
	}
	if _, err := p.Show(ctx, "imdb:tt0100"); err != nil {
		t.Fatalf("imdb lookup: %v", err)
	}
	if logins.Load() != 1 {
		t.Fatalf("logins %d, want 1 (the session is kept)", logins.Load())
	}
	// Without the person's own key TVDB is not used at all.
	key = ""
	p2 := &TVDB{BaseURL: srv.URL + "/v4", Key: func(context.Context) (string, error) { return key, nil }, Gap: time.Millisecond}
	if _, err := p2.Show(ctx, "tvdb:200"); !errors.Is(err, ErrNoTVDBKey) {
		t.Fatalf("no key: %v", err)
	}
	key = "wrong"
	if _, err := p2.SearchShows(ctx, "lab show"); !errors.Is(err, ErrNoTVDBKey) {
		t.Fatalf("refused key: %v", err)
	}
}

func TestShowSourcesPick(t *testing.T) {
	ss := ShowSources{"tmdb": newTMDB(fakeTMDBTV(t), fakeCred)}
	if _, err := ss.Get("tmdb"); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.Get("tvdb"); !errors.Is(err, ErrUnknownSource) {
		t.Fatalf("missing source: %v", err)
	}
	if !ValidShowSource("tvmaze") || ValidShowSource("musicbrainz") {
		t.Fatal("ValidShowSource")
	}
}

// An episode page TVDB cannot find is an ordinary error (the show is not
// gone), and a list longer than the page cap is refused, never cut short.
func TestTVDBEpisodePagesFailLoudly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v4/login":
			_, _ = w.Write([]byte(`{"status":"success","data":{"token":"` + fakeSession + `"}}`))
		case r.URL.Path == "/v4/series/201/extended" || r.URL.Path == "/v4/series/202/extended":
			id := strings.Split(r.URL.Path, "/")[3]
			_, _ = w.Write([]byte(`{"data":{"id":` + id + `,"name":"Lab Show","year":"2025"}}`))
		case r.URL.Path == "/v4/series/201/episodes/default":
			w.WriteHeader(404)
		case r.URL.Path == "/v4/series/202/episodes/default":
			_, _ = w.Write([]byte(`{"data":{"episodes":[{"id":1,"seasonNumber":1,"number":1,"name":"Pilot"}]},"links":{"next":"more"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	p := &TVDB{BaseURL: srv.URL + "/v4", Key: func(context.Context) (string, error) { return fakeTVDBKey, nil }, Gap: time.Microsecond}
	ctx := context.Background()
	if _, err := p.Show(ctx, "tvdb:201"); err == nil || errors.Is(err, ErrShowNotFound) {
		t.Fatalf("episode page 404: %v", err)
	}
	if _, err := p.Show(ctx, "tvdb:202"); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("endless list: %v", err)
	}
	if _, err := p.Show(ctx, "tvdb:999"); !errors.Is(err, ErrShowNotFound) {
		t.Fatalf("unknown show: %v", err)
	}
}
