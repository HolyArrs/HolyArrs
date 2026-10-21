package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// route: GET /api/v1/extras/{kind}/{id}
func TestTitleExtrasFiltersScoresAndFallsBack(t *testing.T) {
	titleExtrasCache.seen = map[string]extrasEntry{}
	asked := 0
	srv := httptest.NewServer(New(Server{Version: "test",
		ExtraArt: func(_ context.Context, kind, id string) (metadata.ServerArt, error) {
			asked++
			if id == "1" {
				return metadata.ServerArt{Logo: "https://assets.fanart.tv/l.png"}, nil
			}
			return metadata.ServerArt{}, metadata.ErrNotFound
		},
		ExtraRatings: func(_ context.Context, kind, id string) (metadata.ServerRatings, error) {
			if id == "3" {
				return metadata.ServerRatings{}, metadata.ErrServiceUnavailable
			}
			return metadata.ServerRatings{Scores: []metadata.ServerScore{
				{Source: "imdb", Value: 8.7, Max: 10, Votes: 2000000},
				{Source: "rt_critics", Value: 83, Max: 100, Votes: 5},
				{Source: "metacritic", Value: 73, Max: 100},
				{Source: "rt_audience", Value: 85, Max: 100, Votes: 250000},
				{Source: "unknown", Value: 1, Max: 10, Votes: 1e9},
			}}, nil
		}}).Handler())
	defer srv.Close()
	var v TitleExtras
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/extras/movie/1", "", 200), &v)
	if v.Logo != "https://assets.fanart.tv/l.png" || len(v.Scores) != 2 || v.Scores[0].Source != "imdb" || v.Scores[1].Source != "rt_audience" {
		t.Fatalf("%+v", v)
	}
	do(t, "GET", srv.URL+"/api/v1/extras/movie/1", "", 200)
	if asked != 1 {
		t.Fatalf("not cached: %d", asked)
	}
	v = TitleExtras{}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/extras/show/2", "", 200), &v)
	if v.Logo != "" || len(v.Scores) != 2 {
		t.Fatalf("show %+v", v)
	}
	// A server hiccup is not kept.
	do(t, "GET", srv.URL+"/api/v1/extras/movie/3", "", 200)
	do(t, "GET", srv.URL+"/api/v1/extras/movie/3", "", 200)
	if asked != 4 {
		t.Fatalf("hiccup cached: %d", asked)
	}
	for _, p := range []string{"/api/v1/extras/movie/01", "/api/v1/extras/album/1", "/api/v1/extras/artist/abc"} {
		do(t, "GET", srv.URL+p, "", 400)
	}
}

func TestTitleExtrasEmptyWhenOff(t *testing.T) {
	srv := httptest.NewServer(New(Server{Version: "test"}).Handler())
	defer srv.Close()
	b := string(do(t, "GET", srv.URL+"/api/v1/extras/movie/9", "", 200))
	if b != "{\"scores\":[]}\n" {
		t.Fatalf("%q", b)
	}
}
