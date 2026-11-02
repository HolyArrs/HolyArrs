package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/anime"
)

type fakeAnime struct{ asked []string }

func (f *fakeAnime) Lookup(_ context.Context, kind string, id int64) (anime.Answer, error) {
	f.asked = append(f.asked, kind)
	switch id {
	case 404:
		return anime.Answer{}, anime.ErrNotFound
	case 503:
		return anime.Answer{}, anime.ErrNoData
	}
	one := 1
	return anime.Answer{Entries: []anime.Entry{{AniDB: 1, AniList: 2, TMDBTV: id, TMDBSeason: &one, TMDBOffset: 12}}, Titles: []anime.Title{{AniList: 2, Romaji: "Gousei"}}, UpdatedAt: "2026-10-21T00:00:00Z"}, nil
}

func TestAnimeMapRoute(t *testing.T) {
	fa := &fakeAnime{}
	s, _ := registering(Options{Registration: true, Burst: 100, Anime: fa, TrustedProxies: testProxy()})
	if w := req(s, "/v1/anime/map?tmdb=1", ""); w.Code != 401 {
		t.Fatalf("no credential %d", w.Code)
	}
	w := req(s, "/v1/anime/map?tmdb=9000", "synthetic-test-token")
	var a anime.Answer
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &a) != nil || len(a.Entries) != 1 || a.Entries[0].TMDBOffset != 12 || a.Titles[0].Romaji != "Gousei" {
		t.Fatalf("map %d %s", w.Code, w.Body)
	}
	tok := registerToken(t, s, "203.0.113.41")
	if w := reqFrom(s, "/v1/anime/map?anilist=5", tok, "203.0.113.41"); w.Code != 200 {
		t.Fatalf("registered %d", w.Code)
	}
	for _, q := range []string{"tmdb=1&kind=movie", "tvdb=1", "mal=1"} {
		if w := req(s, "/v1/anime/map?"+q, "synthetic-test-token"); w.Code != 200 {
			t.Fatalf("%s %d", q, w.Code)
		}
	}
	want := []string{anime.KeyTMDBTV, anime.KeyAniList, anime.KeyTMDBMovie, anime.KeyTVDB, anime.KeyMAL}
	if strings.Join(fa.asked, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds %v", fa.asked)
	}
	if w := req(s, "/v1/anime/map?tmdb=404", "synthetic-test-token"); w.Code != 404 || !strings.Contains(w.Body.String(), "not_anime") {
		t.Fatalf("miss %d", w.Code)
	}
	if w := req(s, "/v1/anime/map?tmdb=503", "synthetic-test-token"); w.Code != 503 {
		t.Fatalf("loading %d", w.Code)
	}
	for _, q := range []string{"", "tmdb=0", "tmdb=01", "tmdb=x", "tmdb=1&tvdb=2", "tvdb=1&kind=movie", "tmdb=1&kind=show", "anidb=1", "tmdb=1&tmdb=2", "tmdb=1&x=1"} {
		if w := req(s, "/v1/anime/map?"+q, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%q accepted %d", q, w.Code)
		}
	}
	s2, _, _ := setup(Options{Burst: 100})
	if w := req(s2, "/v1/anime/map?tmdb=1", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("off %d", w.Code)
	}
}
