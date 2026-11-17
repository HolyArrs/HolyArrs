package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// languageTMDB answers movies 11 and 12 and show 21 with their Spanish
// translations, and season 1 of the show in English and in Spanish.
func languageTMDB(t *testing.T, hits *atomic.Int32) *metadata.TMDB {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"}]}`))
		case "/3/movie/11":
			_, _ = w.Write([]byte(`{"id":11,"title":"Eleven","original_title":"Once","original_language":"es","overview":"English story","tagline":"English tag",
				"release_date":"2024-05-01","genres":[{"name":"Drama"}],
				"recommendations":{"results":[{"id":12,"title":"Twelve","release_date":"2024-05-01","poster_path":"/p.jpg","genre_ids":[18]},
					{"id":41,"title":"Forty-one","release_date":"2024-05-01","poster_path":"/p.jpg","genre_ids":[18]}]},
				"translations":{"translations":[{"iso_639_1":"es","iso_3166_1":"ES","data":{"title":"Once","overview":"Historia","tagline":"Lema"}}]}}`))
		case "/3/movie/12":
			_, _ = w.Write([]byte(`{"id":12,"title":"Twelve","overview":"Twelve story","release_date":"2024-05-01",
				"translations":{"translations":[{"iso_639_1":"es","iso_3166_1":"MX","data":{"title":"Doce","overview":""}}]}}`))
		case "/3/tv/21":
			_, _ = w.Write([]byte(`{"id":21,"name":"Twenty-one","overview":"Show story","seasons":[{"season_number":1,"episode_count":2}]}`))
		case "/3/tv/21/season/1":
			if r.URL.Query().Get("language") == "es-ES" {
				_, _ = w.Write([]byte(`{"episodes":[{"episode_number":1,"name":"Piloto","overview":"Empieza"},{"episode_number":2,"name":"Episodio 2","overview":""}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"episodes":[{"episode_number":1,"name":"Pilot","overview":"It starts","runtime":42},{"episode_number":2,"name":"Two","overview":"It goes on"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

// askIn asks url with the given request headers and answers the body.
func askIn(t *testing.T, url string, header map[string]string) ([]byte, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%s: %d %s", url, resp.StatusCode, b)
	}
	return b, resp.Header
}

type titleTextAnswer struct {
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	Tagline       string `json:"tagline"`
	OriginalTitle string `json:"original_title"`
	Similar       []struct {
		TMDBID string `json:"tmdb_id"`
		Name   string `json:"name"`
	} `json:"similar"`
}

func titleText(t *testing.T, url string, header map[string]string) titleTextAnswer {
	t.Helper()
	b, _ := askIn(t, url, header)
	var a titleTextAnswer
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

// route: GET /api/v1/metadata/{kind}/{tmdb_id}
func TestMetadataDetailsInPersonsLanguage(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv, _ := discoverRig(t, languageTMDB(t, &hits))
	url := srv.URL + "/api/v1/metadata/movie/11"
	en := titleText(t, url, nil)
	if en.Name != "Eleven" || en.Overview != "English story" || en.Tagline != "English tag" || en.OriginalTitle != "Once" {
		t.Fatalf("English page %+v", en)
	}
	es := titleText(t, url, map[string]string{"X-HolyArrs-Language": "es"})
	// The original title equals the shown name, so it is not repeated.
	if es.Name != "Once" || es.Overview != "Historia" || es.Tagline != "Lema" || es.OriginalTitle != "" {
		t.Fatalf("Spanish page %+v", es)
	}
	// The browser's language counts when the app sends none.
	if b := titleText(t, url, map[string]string{"Accept-Language": "es-MX,es;q=0.9"}); b.Name != "Once" {
		t.Fatalf("browser Spanish %+v", b)
	}
	// No German text: English, and nothing asked outside again.
	before := hits.Load()
	if de := titleText(t, url, map[string]string{"X-HolyArrs-Language": "de"}); de.Name != "Eleven" || de.Overview != "English story" || de.OriginalTitle != "Once" {
		t.Fatalf("German page %+v", de)
	}
	if hits.Load() != before {
		t.Fatalf("a kept page asked outside again: %d -> %d", before, hits.Load())
	}
	// The recommended title shows its Spanish name once its own page was read.
	titleText(t, srv.URL+"/api/v1/metadata/movie/12", nil)
	es = titleText(t, url, map[string]string{"X-HolyArrs-Language": "es"})
	if len(es.Similar) != 2 || es.Similar[0].Name != "Doce" || es.Similar[1].Name != "Forty-one" {
		t.Fatalf("similar row %+v", es.Similar)
	}
	if en := titleText(t, url, nil); en.Similar[0].Name != "Twelve" {
		t.Fatalf("English similar row %+v", en.Similar)
	}
}

// A page kept before titles carried their texts is read again for a
// non-English person, so they see their language at once.
func TestMetadataDetailsKeptWithoutTexts(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv, s := discoverRig(t, languageTMDB(t, &hits))
	metadata.KeepOnDisk(context.Background(), s, "details|movie|11", metadata.Details{DiscoverItem: metadata.DiscoverItem{TMDBID: "11", Kind: "movie", Name: "Eleven", Overview: "English story", Genres: []string{}}})
	url := srv.URL + "/api/v1/metadata/movie/11"
	if en := titleText(t, url, nil); en.Name != "Eleven" || hits.Load() != 0 {
		t.Fatalf("English from the kept page: %+v, %d asks", en, hits.Load())
	}
	if es := titleText(t, url, map[string]string{"X-HolyArrs-Language": "es"}); es.Name != "Once" || es.Overview != "Historia" {
		t.Fatalf("Spanish page %+v", es)
	}
}

// Reading a kept page again for its texts never costs the page: a refused
// or missing answer still shows the kept one, in English.
func TestMetadataDetailsKeptWithoutTextsSourceRefuses(t *testing.T) {
	t.Parallel()
	for _, status := range []int{401, 404} {
		// A fresh kept page, and one a day old (due to be read again anyway).
		for _, age := range []time.Duration{0, 48 * time.Hour} {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			t.Cleanup(srv.Close)
			p := &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
			api, s := discoverRig(t, p)
			at := time.Now().Add(-age)
			b, _ := json.Marshal(metadata.Kept[metadata.Details]{V: metadata.Details{DiscoverItem: metadata.DiscoverItem{TMDBID: "11", Kind: "movie", Name: "Eleven", Genres: []string{}},
				Videos: []metadata.DetailVideo{{Key: "abcdefgh"}}}, At: at})
			if err := (metadata.LookupCache{Store: s}).Put(context.Background(), "details|movie|11", b, at, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if es := titleText(t, api.URL+"/api/v1/metadata/movie/11", map[string]string{"X-HolyArrs-Language": "es"}); es.Name != "Eleven" {
				t.Fatalf("%d, %v old: kept page %+v", status, age, es)
			}
		}
	}
}

// route: GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}
func TestMetadataSeasonInPersonsLanguage(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv, _ := discoverRig(t, languageTMDB(t, &hits))
	url := srv.URL + "/api/v1/metadata/show/21/seasons/1"
	type episodes struct {
		Episodes []metadata.DetailEpisode `json:"episodes"`
	}
	read := func(header map[string]string) []metadata.DetailEpisode {
		b, h := askIn(t, url, header)
		if !strings.Contains(h.Get("Vary"), "X-HolyArrs-Language") {
			t.Errorf("Vary %q", h.Get("Vary"))
		}
		var e episodes
		_ = json.Unmarshal(b, &e)
		return e.Episodes
	}
	es := read(map[string]string{"X-HolyArrs-Language": "es"})
	if len(es) != 2 || es[0].Name != "Piloto" || es[0].Overview != "Empieza" || es[0].Runtime != 42 || es[1].Name != "Episodio 2" || es[1].Overview != "It goes on" {
		t.Fatalf("Spanish season %+v", es)
	}
	if en := read(nil); en[0].Name != "Pilot" || en[0].Overview != "It starts" {
		t.Fatalf("English season %+v", en)
	}
}
