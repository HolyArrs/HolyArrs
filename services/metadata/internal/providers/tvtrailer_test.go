package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// A show with no trailer of its own gets its newest season's trailer
// (Grey's Anatomy, TMDB tv 1416), official and English first.
func TestShowWithoutTrailerTakesNewestSeasonTrailer(t *testing.T) {
	var seasons atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/tv/1416":
			_, _ = w.Write([]byte(`{"id":1416,"seasons":[{"season_number":0},{"season_number":20},{"season_number":21},{"season_number":22}],"videos":{"results":[{"key":"clip","site":"YouTube","type":"Clip"}]}}`))
		case "/3/tv/1416/season/22/videos":
			seasons.Add(1)
			_, _ = w.Write([]byte(`{"results":[]}`))
		case "/3/tv/1416/season/21/videos":
			seasons.Add(1)
			_, _ = w.Write([]byte(`{"results":[{"key":"fanmade","site":"YouTube","type":"Trailer","official":false,"iso_639_1":"en"},{"key":"s21official","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"}]}`))
		default:
			seasons.Add(1)
			_, _ = w.Write([]byte(`{"results":[{"key":"other","site":"YouTube","type":"Trailer","official":true}]}`))
		}
	}))
	defer up.Close()
	c := tmdbTestClient(t, up.URL+"/3")
	q := model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: "/tv/1416", Params: "append_to_response=videos", Page: 1, Limit: 20}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := c.Fetch(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Data), `"key":"s21official"`) || seasons.Load() != 2 || TVNeedsTrailer(q, r.Data) {
		t.Fatalf("seasons %d %s", seasons.Load(), r.Data)
	}
	// Nothing anywhere: still needs one (kept briefly by the server).
	if !TVNeedsTrailer(q, []byte(`{"videos":{"results":[]}}`)) || TVNeedsTrailer(model.Query{Operation: model.OperationTMDB, Path: "/movie/1", Params: "append_to_response=videos"}, []byte(`{}`)) {
		t.Fatal("needs-trailer rule")
	}
}

func tmdbTestClient(t *testing.T, base string) *Client {
	t.Helper()
	c, err := New(Options{Providers: map[string]Config{"tmdb": {BaseURL: base, Key: "synthetic-tmdb-key", Interval: time.Millisecond}}, AllowPrivate: true, Timeout: 5 * time.Second, Attempts: 1, UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
