package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type rqReviewsView struct {
	Source  string `json:"source"`
	Sources []struct {
		ID     string   `json:"id"`
		Orders []string `json:"orders"`
	} `json:"sources"`
	Order        string  `json:"order"`
	Score        float64 `json:"score"`
	Votes        int     `json:"votes"`
	Distribution []int   `json:"distribution"`
	Watching     *int    `json:"watching"`
	Reviews      []struct {
		Author string `json:"author"`
		Text   string `json:"text"`
	} `json:"reviews"`
	Pages  int  `json:"pages"`
	Hidden bool `json:"reviews_hidden"`
}

// route: GET /api/v1/metadata/{kind}/{tmdb_id}/reviews
// route: GET /api/v1/settings/trakt-token
// route: PUT /api/v1/settings/trakt-token
func TestMetadataReviews(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	key, _ := secrets.LoadKey(dir, "")
	v, err := secrets.Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	var tmdbHits atomic.Int32
	tm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tmdbHits.Add(1)
		switch r.URL.Path {
		case "/3/genre/movie/list":
			_, _ = w.Write([]byte(`{"genres":[]}`))
		case "/3/movie/11":
			_, _ = w.Write([]byte(`{"id":11,"title":"M11","release_date":"2024-05-01","vote_average":7.84,"vote_count":12400}`))
		case "/3/movie/11/reviews":
			_, _ = w.Write([]byte(`{"page":1,"total_pages":1,"results":[{"author":"ann","author_details":{"rating":7},"content":"<p>Good</p>","created_at":"2024-02-01T00:00:00Z"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(tm.Close)
	tk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer VIP_FOR_TEST_ONLY" {
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/search/tmdb/11":
			_, _ = w.Write([]byte(`[{"type":"movie","movie":{"ids":{"trakt":3}}}]`))
		case "/movies/3/ratings":
			_, _ = w.Write([]byte(`{"rating":8.5,"votes":40,"distribution":{"9":40}}`))
		case "/movies/3/watching":
			_, _ = w.Write([]byte(`[{}]`))
		case "/movies/3/comments/newest":
			_, _ = w.Write([]byte(`[{"comment":"Nice","user":{"username":"zed"}}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(tk.Close)
	prov := &metadata.TMDB{BaseURL: tm.URL + "/3", Access: func(context.Context) (string, error) { return "x", nil }, Gap: time.Millisecond}
	srv := httptest.NewServer(buildHandlers(services{Store: s, Metadata: prov, Version: "test", Secrets: v, TraktBase: tk.URL}).Handler())
	t.Cleanup(srv.Close)
	get := func(path string) rqReviewsView {
		var out rqReviewsView
		if err := json.Unmarshal(rqDo(t, "GET", srv.URL+path, "", 200), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// TMDB alone: score from the details, plain-text reviews, no Trakt pill.
	g := get("/api/v1/metadata/movie/11/reviews")
	if g.Source != "tmdb" || len(g.Sources) != 1 || g.Score != 7.8 || g.Votes != 12400 || len(g.Reviews) != 1 || g.Reviews[0].Text != "Good" || g.Order != "newest" {
		t.Fatalf("tmdb %+v", g)
	}
	before := tmdbHits.Load()
	get("/api/v1/metadata/movie/11/reviews")
	if tmdbHits.Load() != before {
		t.Fatal("reviews not cached")
	}
	if d := string(rqDo(t, "GET", srv.URL+"/api/v1/metadata/movie/11", "", 200)); !strings.Contains(d, `"score":7.8`) || !strings.Contains(d, `"votes":12400`) {
		t.Fatalf("details score %s", d)
	}
	rqDo(t, "GET", srv.URL+"/api/v1/metadata/movie/11/reviews?source=trakt", "", 404)
	rqDo(t, "GET", srv.URL+"/api/v1/metadata/movie/11/reviews?source=imdb", "", 400)
	rqDo(t, "GET", srv.URL+"/api/v1/metadata/movie/11/reviews?page=0", "", 400)
	rqDo(t, "GET", srv.URL+"/api/v1/metadata/movie/11/reviews?order=best", "", 400)
	rqDo(t, "GET", srv.URL+"/api/v1/metadata/artist/11/reviews", "", 400)

	// The client id alone is not enough; the VIP token turns Trakt on.
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/trakt-key", `{"key":"TRAKT_FOR_TEST_ONLY"}`, 200)
	if g := get("/api/v1/metadata/movie/11/reviews"); len(g.Sources) != 1 {
		t.Fatalf("trakt without token %+v", g)
	}
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/trakt-token", `{}`, 400)
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/trakt-token", `{"token":"VIP_FOR_TEST_ONLY"}`, 200)
	if b := string(rqDo(t, "GET", srv.URL+"/api/v1/settings/trakt-token", "", 200)); strings.Contains(b, "VIP_FOR_TEST_ONLY") || !strings.Contains(b, secrets.Masked) {
		t.Fatalf("token shown: %s", b)
	}
	g = get("/api/v1/metadata/movie/11/reviews?source=trakt")
	if g.Source != "trakt" || len(g.Sources) != 2 || g.Sources[1].ID != "trakt" || len(g.Sources[1].Orders) != 2 || g.Score != 8.5 || g.Votes != 40 ||
		g.Watching == nil || *g.Watching != 1 || len(g.Distribution) != 10 || g.Distribution[8] != 40 || len(g.Reviews) != 1 || g.Reviews[0].Author != "zed" {
		t.Fatalf("trakt %+v", g)
	}
	// A refused token says so.
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/trakt-token", `{"token":"OLD_FOR_TEST_ONLY"}`, 200)
	rqDo(t, "GET", srv.URL+"/api/v1/metadata/movie/11/reviews?source=trakt&order=helpful", "", 502)
}
