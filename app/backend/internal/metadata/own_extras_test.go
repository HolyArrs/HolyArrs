package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOwnExtrasWithOwnKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tmdb/movie/603":
			if r.URL.Query().Get("apikey") != "mdb-fixture" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"ids":{"imdb":"tt0133093"},"ratings":[{"source":"imdb","value":8.7,"votes":2000000},{"source":"tomatoes","value":83,"votes":150},{"source":"popcorn","value":85,"votes":25000},{"source":"letterboxd","value":4.2,"votes":900000},{"source":"odd","value":5},{"source":"metacritic","value":null}]}`))
		case r.URL.Path == "/v3/movies/603":
			if r.Header.Get("api-key") != "fan-fixture" || r.URL.RawQuery != "" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"hdmovielogo":[{"url":"https://assets.fanart.tv/fanart/movies/603/hdmovielogo/x.png","lang":"de"},{"url":"https://assets.fanart.tv/fanart/movies/603/hdmovielogo/y.png","lang":"en"}],
				"moviebackground":[{"url":"https://assets.fanart.tv/fanart/movies/603/moviebackground/a.jpg"},{"url":"https://evil.example/b.jpg"},{"url":"javascript:alert(1)"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	mdb, fan := "mdb-fixture", "fan-fixture"
	o := &OwnExtras{MDBListBase: srv.URL, FanartBase: srv.URL + "/v3",
		MDBListKey: func(context.Context) (string, error) { return mdb, nil },
		FanartKey:  func(context.Context) (string, error) { return fan, nil }}
	ctx := context.Background()
	rt, err := o.Ratings(ctx, "movie", "603")
	if err != nil || rt.IMDbID != "tt0133093" || len(rt.Scores) != 4 {
		t.Fatalf("%+v %v", rt, err)
	}
	if rt.Scores[1].Source != "rt_critics" || rt.Scores[2].Source != "rt_audience" || rt.Scores[3].Max != 5 {
		t.Fatalf("names and tops: %+v", rt.Scores)
	}
	art, err := o.Art(ctx, "movie", "603")
	if err != nil || !strings.HasSuffix(art.Logo, "/y.png") || len(art.Backgrounds) != 1 {
		t.Fatalf("%+v %v", art, err)
	}
	if _, err := o.Art(ctx, "show", "603"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("shows have no fanart.tv art by TMDB id: %v", err)
	}
	mdb = "wrong"
	if _, err := o.Ratings(ctx, "movie", "603"); !errors.Is(err, ErrNoAccess) || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("a refused key: %v", err)
	}
	mdb, fan = "", ""
	if _, err := o.Ratings(ctx, "movie", "603"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no key: %v", err)
	}
	if _, err := o.Art(ctx, "movie", "603"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no key: %v", err)
	}
}

func TestModeExtrasPick(t *testing.T) {
	m := NewModes(envOf(nil))
	server := func(context.Context, string, string) (ServerRatings, error) {
		return ServerRatings{IMDbID: "server"}, nil
	}
	own := func(context.Context, string, string) (ServerRatings, error) { return ServerRatings{IMDbID: "own"}, nil }
	pick := ModeRatings(m, server, own)
	ctx := context.Background()
	if r, _ := pick(ctx, "movie", "1"); r.IMDbID != "server" {
		t.Fatal(r.IMDbID)
	}
	_ = m.Set(ModeOwn)
	if r, _ := pick(ctx, "movie", "1"); r.IMDbID != "own" {
		t.Fatal(r.IMDbID)
	}
	_ = m.Set(ModeOff)
	if _, err := pick(ctx, "movie", "1"); !SourceOff(err) {
		t.Fatal(err)
	}
}
