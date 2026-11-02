package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

func TestTMDBReadsAllowlistedRawAndCached(t *testing.T) {
	s, _ := registering(Options{Registration: true, Burst: 100, TrustedProxies: testProxy()})
	var got []model.Query
	s.fetcher = fetchFunc(func(_ context.Context, q model.Query) (model.Result, error) {
		got = append(got, q)
		return model.Result{Data: json.RawMessage(`{"page":1,"results":[{"id":7}]}`), Complete: true}, nil
	})
	tok := registerToken(t, s, "203.0.113.60")
	for _, path := range []string{"/v1/tmdb/trending/movie/week?language=en-US", "/v1/tmdb/discover/tv?with_genres=18&sort_by=popularity.desc&include_adult=false&language=en-US", "/v1/tmdb/person/31/movie_credits", "/v1/tmdb/genre/movie/list?language=en-US", "/v1/tmdb/certification/movie/list", "/v1/tmdb/movie/550/recommendations", "/v1/tmdb/find/tt0903747?external_source=imdb_id", "/v1/tmdb/list/1?page=2"} {
		w := reqFrom(s, path, tok, "203.0.113.60")
		if w.Code != 200 || w.Body.String() != `{"page":1,"results":[{"id":7}]}` {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := reqFrom(s, "/v1/tmdb/trending/movie/week?language=en-US", tok, "203.0.113.60"); w.Header().Get("X-Metadata-Cache") != "hit" {
		t.Fatal("raw read not cached")
	}
	if len(got) != 8 || got[1].Path != "/discover/tv" || got[1].Params != "include_adult=false&language=en-US&sort_by=popularity.desc&with_genres=18" {
		t.Fatalf("queries %+v", got)
	}
	for _, path := range []string{"/v1/tmdb/account/1", "/v1/tmdb/movie/550?api_key=x", "/v1/tmdb/movie/../account", "/v1/tmdb/trending/movie/week?page=0", "/v1/tmdb/search/movie?query=a&query=b", "/v1/tmdb/authentication/token/new"} {
		if w := reqFrom(s, path, tok, "203.0.113.60"); w.Code != 400 {
			t.Fatalf("%s accepted: %d", path, w.Code)
		}
	}
	if w := reqFrom(s, "/v1/tmdb/trending/movie/week", "", "203.0.113.60"); w.Code != 401 {
		t.Fatalf("raw read without credential %d", w.Code)
	}
}
