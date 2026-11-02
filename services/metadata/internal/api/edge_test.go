package api

import (
	"strings"
	"testing"
)

// Every route has a cache class: shared catalogue routes are public on
// success and answer without a credential; per-install, search, stats and
// TMDB routes keep no public policy.
func TestEdgeRouteClasses(t *testing.T) {
	public := []string{
		"/v1/control", "/v1/lists/music-trending", "/v1/lists/service-top", "/v1/music/charts", "/v1/music/charts/rising", "/v1/music/new", "/v1/music/lists/shelves",
		"/v1/music/genres", "/v1/music/genres/tile/rock-0.jpg", "/v1/music/musicbrainz/artist/x", "/v1/music/musicbrainz/release-group/x",
		"/v1/music/album/x", "/v1/music/similar/x", "/v1/music/cover/x", "/v1/music/videos/x", "/v1/art/music/x", "/v1/img/album/x/500.jpg",
	}
	for _, p := range public {
		if !strings.HasPrefix(edgePolicy(p), "public") {
			t.Fatalf("%s not public", p)
		}
	}
	private := []string{
		"/v1/search", "/v1/stats", "/metrics", "/v1/installations", "/v1/tmdb/movie/1", "/v1/movies/tmdb/1",
		"/v1/music/similar-set", "/v1/music/top-songs/x", "/v1/music/discography/x", "/v1/music/album-details", "/v1/music/artist-data", "/v1/music/suggest", "/v1/music/releases",
		"/v1/trailers/movie/1", "/v1/wikidata/movie/1", "/v1/ratings/movie/1", "/v1/anime/map",
	}
	for _, p := range private {
		if edgePolicy(p) != "" {
			t.Fatalf("%s is public", p)
		}
	}
}

func TestEdgeHeadersAndErrors(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, Music: fakeMusic{"artist|global": `{"a":1}`}})
	w := req(s, "/v1/lists/music-trending?kind=artist", "")
	cc := w.Header().Get("Cache-Control")
	if w.Code != 200 || !strings.Contains(cc, "s-maxage=21600") || !strings.Contains(cc, "stale-while-revalidate") || w.Header().Get("Vary") != "" {
		t.Fatalf("anonymous shared list %d %q", w.Code, cc)
	}
	if w := req(s, "/v1/lists/music-trending?kind=nope", ""); w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("error not no-store %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	if w := req(s, "/v1/lists/music-trending?kind=artist", "wrong-token"); w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("bad credential %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	if w := req(s, "/v1/search?provider=musicbrainz&kind=artist&q=x", ""); w.Code != 401 {
		t.Fatalf("search without a credential %d", w.Code)
	}
}

func TestControlRoute(t *testing.T) {
	s, _, calls := setup(Options{Burst: 100})
	w := req(s, "/v1/control", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"prefetch":"on"`) || !strings.Contains(w.Header().Get("Cache-Control"), "s-maxage=60") || w.Header().Get("ETag") == "" {
		t.Fatalf("control %d %v %s", w.Code, w.Header(), w.Body)
	}
	if w := req(s, "/v1/control?x=1", ""); w.Code != 400 {
		t.Fatalf("query accepted %d", w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("control reached a provider")
	}
}
