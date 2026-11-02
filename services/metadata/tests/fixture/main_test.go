package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
)

func TestPaddingSurvivesProviderNormalization(t *testing.T) {
	h := newFixture()
	s := httptest.NewServer(h)
	defer s.Close()
	client, err := providers.New(providers.Options{Providers: map[string]providers.Config{"tmdb": {BaseURL: s.URL + "/tmdb/3", Interval: time.Nanosecond}}, AllowPrivate: true, Attempts: 1, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	q := model.Query{Provider: "tmdb", Kind: "movie", Operation: "detail", ID: "93", Page: 1, Limit: 20}
	plain, err := client.Fetch(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	request(t, h, "POST", "/control", `{"payload_bytes":4096}`)
	padded, err := client.Fetch(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	var normalized map[string]interface{}
	if err := json.Unmarshal(padded.Data, &normalized); err != nil {
		t.Fatal(err)
	}
	if len(padded.Data) < len(plain.Data)+4096 || len(normalized["summary"].(string)) < 4096 {
		t.Fatal("dataset payload must increase normalized and retained metadata bytes")
	}
}

func TestSearchPagination(t *testing.T) {
	h := newFixture()
	t.Run("tmdb", func(t *testing.T) {
		var tmdb map[string]interface{}
		json.Unmarshal(request(t, h, "GET", "/tmdb/3/search/movie?page=2", "").Body.Bytes(), &tmdb)
		if tmdb["page"] != float64(2) || tmdb["total_pages"].(float64) < 2 {
			t.Fatal("inconsistent TMDB pagination")
		}
	})
	t.Run("musicbrainz", func(t *testing.T) {
		var mb map[string]interface{}
		json.Unmarshal(request(t, h, "GET", "/musicbrainz/ws/2/artist?query=synthetic&offset=20&limit=7", "").Body.Bytes(), &mb)
		if mb["offset"] != float64(20) || len(mb["artists"].([]interface{})) != 7 || mb["count"].(float64) < 27 {
			t.Fatal("inconsistent MusicBrainz pagination")
		}
	})
}

func request(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
	return r
}

func TestSyntheticProviderShapes(t *testing.T) {
	h := newFixture()
	cases := []struct{ path, field string }{
		{"/tmdb/3/search/movie?query=synthetic", "results"},
		{"/tmdb/3/search/tv?query=synthetic", "results"},
		{"/tmdb/3/movie/23", "id"},
		{"/tmdb/3/tv/23", "seasons"},
		{"/tmdb/3/tv/23/season/0", "episodes"},
		{"/tvmaze/shows/23", "id"},
		{"/musicbrainz/ws/2/artist?query=synthetic", "artists"},
		{"/musicbrainz/ws/2/release-group?query=synthetic", "release-groups"},
		{"/musicbrainz/ws/2/release?query=synthetic", "releases"},
		{"/musicbrainz/ws/2/recording?query=synthetic", "recordings"},
		{"/musicbrainz/ws/2/release/00000000-0000-4000-8000-000000000023", "media"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			r := request(t, h, "GET", c.path, "")
			if r.Code != 200 {
				t.Fatalf("status %d", r.Code)
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(r.Body.Bytes(), &obj); err != nil {
				t.Fatal(err)
			}
			if _, ok := obj[c.field]; !ok {
				t.Fatalf("missing %s", c.field)
			}
		})
	}
	r := request(t, h, "GET", "/tvmaze/search/shows?q=synthetic", "")
	var shows []struct {
		Show struct {
			ID int `json:"id"`
		} `json:"show"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &shows); err != nil || len(shows) != 1 || shows[0].Show.ID < 1 {
		t.Fatal("invalid TVmaze search")
	}
}

func TestTMDBAdvertisedSeasonIDsMatchSeasonPayloads(t *testing.T) {
	h := newFixture()
	var show struct {
		NumberOfSeasons  int `json:"number_of_seasons"`
		NumberOfEpisodes int `json:"number_of_episodes"`
		Seasons          []struct {
			ID           int `json:"id"`
			SeasonNumber int `json:"season_number"`
			EpisodeCount int `json:"episode_count"`
		} `json:"seasons"`
	}
	if err := json.Unmarshal(request(t, h, "GET", "/tmdb/3/tv/23", "").Body.Bytes(), &show); err != nil {
		t.Fatal(err)
	}
	if show.NumberOfSeasons != 1 || len(show.Seasons) != 2 {
		t.Fatalf("expected one numbered season plus specials, got number_of_seasons=%d and %d advertised seasons", show.NumberOfSeasons, len(show.Seasons))
	}

	advertisedEpisodes := 0
	for _, advertised := range show.Seasons {
		advertisedEpisodes += advertised.EpisodeCount
		var season struct {
			ID           int           `json:"id"`
			SeasonNumber int           `json:"season_number"`
			Episodes     []interface{} `json:"episodes"`
		}
		path := fmt.Sprintf("/tmdb/3/tv/23/season/%d", advertised.SeasonNumber)
		response := request(t, h, "GET", path, "")
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned status %d", path, response.Code)
		}
		if err := json.Unmarshal(response.Body.Bytes(), &season); err != nil {
			t.Fatal(err)
		}
		if season.ID != advertised.ID {
			t.Errorf("season %d advertised id %d, route returned id %d", advertised.SeasonNumber, advertised.ID, season.ID)
		}
		if season.SeasonNumber != advertised.SeasonNumber {
			t.Errorf("season route %d returned season_number %d", advertised.SeasonNumber, season.SeasonNumber)
		}
		if len(season.Episodes) != advertised.EpisodeCount {
			t.Errorf("season %d advertised %d episodes, route returned %d", advertised.SeasonNumber, advertised.EpisodeCount, len(season.Episodes))
		}
	}
	if advertisedEpisodes != show.NumberOfEpisodes {
		t.Errorf("advertised seasons total %d episodes, show declared %d", advertisedEpisodes, show.NumberOfEpisodes)
	}
}

func TestFaultControlAndStats(t *testing.T) {
	h := newFixture()
	request(t, h, "POST", "/control", `{"route":"/tmdb/3/movie/7","status":429,"retry_after":"2","delay_ms":15}`)
	start := time.Now()
	r := request(t, h, "GET", "/tmdb/3/movie/7", "")
	if r.Code != 429 || r.Header().Get("Retry-After") != "2" || time.Since(start) < 15*time.Millisecond {
		t.Fatal("fault control not applied")
	}
	if request(t, h, "GET", "/tmdb/3/movie/8", "").Code != 200 {
		t.Fatal("route control leaked")
	}
	r = request(t, h, "GET", "/stats", "")
	var stats struct {
		Total  int `json:"total"`
		Routes map[string]struct {
			Count      int      `json:"count"`
			Timestamps []string `json:"timestamps"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 || stats.Routes["/tmdb/3/movie/7"].Count != 1 || len(stats.Routes["/tmdb/3/movie/7"].Timestamps) != 1 {
		t.Fatal("incorrect counters")
	}
	request(t, h, "POST", "/control", `{"reset":true}`)
	if request(t, h, "GET", "/tmdb/3/movie/7", "").Code != 200 {
		t.Fatal("reset failed")
	}
}

func TestInvalidBodiesAndDeterministicPadding(t *testing.T) {
	for _, mode := range []string{"malformed", "truncated", "incomplete", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			h := newFixture()
			request(t, h, "POST", "/control", `{"mode":"`+mode+`"}`)
			r := request(t, h, "GET", "/tmdb/3/movie/1", "")
			var obj map[string]interface{}
			err := json.Unmarshal(r.Body.Bytes(), &obj)
			if (mode == "malformed" || mode == "truncated") && err == nil {
				t.Fatal("expected invalid JSON")
			}
			if mode == "incomplete" && obj["id"] != nil {
				t.Fatal("expected missing identity")
			}
			if mode == "oversized" && r.Body.Len() <= 2*1024*1024 {
				t.Fatal("expected body above default cap")
			}
		})
	}
	h := newFixture()
	request(t, h, "POST", "/control", `{"payload_bytes":4096}`)
	a := request(t, h, "GET", "/tmdb/3/movie/92", "")
	b := request(t, h, "GET", "/tmdb/3/movie/92", "")
	if a.Body.String() != b.Body.String() || a.Body.Len() < 4096 {
		t.Fatal("padding must be deterministic")
	}
	var padded map[string]interface{}
	if err := json.Unmarshal(a.Body.Bytes(), &padded); err != nil {
		t.Fatal(err)
	}
	if overview, ok := padded["overview"].(string); !ok || len(overview) < 4096 {
		t.Fatal("padding must grow the overview retained in normalized metadata")
	}
	for _, p := range []string{"/tmdb/3/movie/0", "/tmdb/3/movie/no", "/musicbrainz/ws/2/artist/invalid"} {
		if request(t, h, "GET", p, "").Code != 404 {
			t.Fatalf("accepted invalid path %s", p)
		}
	}
}
