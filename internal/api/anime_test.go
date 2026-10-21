package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

func animeTMDB(t *testing.T) metadata.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":16,"name":"Animation"}]}`))
		case "/3/tv/9000":
			_, _ = w.Write([]byte(`{"id":9000,"name":"Synthetic Show","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"original_language":"ja",
				"seasons":[{"season_number":1,"episode_count":12},{"season_number":2,"episode_count":24}]}`))
		case "/3/tv/9100":
			_, _ = w.Write([]byte(`{"id":9100,"name":"Other Cartoon","genres":[{"id":16,"name":"Animation"}],"origin_country":["JP"],"original_language":"ja"}`))
		case "/3/tv/9200":
			_, _ = w.Write([]byte(`{"id":9200,"name":"Drama","genres":[],"origin_country":["US"],"original_language":"en"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

// route: GET /api/v1/metadata/{kind}/{tmdb_id}/anime
func TestMetadataAnime(t *testing.T) {
	md := animeTMDB(t)
	_, st := discoverRig(t, md)
	one, two := 1, 2
	var asked []string
	srv := httptest.NewServer(New(Server{Store: st, Metadata: md, Version: "test", AnimeMap: func(_ context.Context, by, id string) (metadata.AnimeMap, error) {
		asked = append(asked, by+":"+id)
		if id != "9000" {
			return metadata.AnimeMap{}, metadata.ErrNotFound
		}
		return metadata.AnimeMap{Entries: []metadata.AnimeEntry{{AniList: 11, TMDBTV: 9000, TMDBSeason: &one}, {AniList: 12, TMDBTV: 9000, TMDBSeason: &two}},
			Titles: []metadata.AnimeTitle{{AniList: 11, Romaji: "Gousei Shou", English: "Synthetic Show", Native: "合成ショー"}}}, nil
	}}).Handler())
	t.Cleanup(srv.Close)
	get := func(path string) animeView {
		var v animeView
		if err := json.Unmarshal(do(t, "GET", srv.URL+path, "", 200), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, id := range []string{"9000", "9100", "9200"} {
		do(t, "GET", srv.URL+"/api/v1/metadata/show/"+id, "", 200)
	}
	v := get("/api/v1/metadata/show/9000/anime")
	if !v.Anime || !v.Mapped || len(v.AltNames) != 2 || v.AltNames[0] != "Gousei Shou" || v.AltNames[1] != "合成ショー" || len(v.Seasons) != 2 || v.Seasons[1].AbsoluteFrom != 13 {
		t.Fatalf("mapped %+v", v)
	}
	// No mapping: TMDB's genre and origin decide.
	if v := get("/api/v1/metadata/show/9100/anime"); !v.Anime || v.Mapped || len(v.Seasons) != 0 || v.AltNames == nil {
		t.Fatalf("looks anime %+v", v)
	}
	if v := get("/api/v1/metadata/show/9200/anime"); v.Anime {
		t.Fatalf("not anime %+v", v)
	}
	get("/api/v1/metadata/movie/9000/anime")
	if asked[len(asked)-1] != "tmdb_movie:9000" || asked[0] != "tmdb_tv:9000" {
		t.Fatalf("asked %v", asked)
	}
	do(t, "GET", srv.URL+"/api/v1/metadata/artist/1/anime", "", 400)
	do(t, "GET", srv.URL+"/api/v1/metadata/show/01/anime", "", 400)
}
