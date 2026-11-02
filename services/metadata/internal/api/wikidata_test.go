package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/wikidata"
)

type fakeWikidata struct{}

func (fakeWikidata) Title(_ context.Context, kind string, id int64) (wikidata.Entity, error) {
	if id == 404 {
		return wikidata.Entity{}, wikidata.ErrNotFound
	}
	return wikidata.Entity{QID: "Q1", Kind: kind, Tallies: []wikidata.Tally{{Family: "oscar", Wins: 2}}}, nil
}
func (fakeWikidata) Person(context.Context, int64) (wikidata.Entity, error) {
	return wikidata.Entity{QID: "Q2", Kind: "person"}, nil
}
func (fakeWikidata) TVmazePhoto(_ context.Context, _ int64, name string) string {
	if name == "Joanna van Kampen" {
		return "https://static.tvmaze.com/uploads/images/medium_portrait/293/732996.jpg"
	}
	return ""
}
func (fakeWikidata) Artist(context.Context, string) (wikidata.Entity, error) {
	return wikidata.Entity{QID: "Q3", Kind: "artist"}, nil
}
func (fakeWikidata) Winners(kind string) ([]wikidata.Winner, bool) {
	return []wikidata.Winner{{TMDB: 13, Wins: 6}}, kind == "movie"
}
func (fakeWikidata) Stats() wikidata.Stats { return wikidata.Stats{Movies: 1} }

func TestWikidataRoutes(t *testing.T) {
	s, _ := registering(Options{Registration: true, Burst: 100, Wikidata: fakeWikidata{}, TrustedProxies: testProxy()})
	if w := req(s, "/v1/wikidata/person/31", ""); w.Code != 401 {
		t.Fatalf("no credential %d", w.Code)
	}
	var e wikidata.Entity
	w := req(s, "/v1/wikidata/title/movie/13", "synthetic-test-token")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &e) != nil || e.QID != "Q1" || e.Tallies[0].Wins != 2 {
		t.Fatalf("title %d %s", w.Code, w.Body)
	}
	// Self-registered credentials reach every Wikidata route.
	tok := registerToken(t, s, "203.0.113.41")
	for _, p := range []string{"/v1/wikidata/person/31", "/v1/wikidata/title/tv/1396", "/v1/wikidata/artist/5b11f4ce-a62d-471e-81fc-a69a8278c7da", "/v1/wikidata/award-winners?kind=movie"} {
		if w := reqFrom(s, p, tok, "203.0.113.41"); w.Code != 200 {
			t.Fatalf("%s %d", p, w.Code)
		}
	}
	// A person's name brings the TVmaze photo when Wikidata has none.
	w = req(s, "/v1/wikidata/person/31?name=Joanna%20van%20Kampen", "synthetic-test-token")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &e) != nil || !strings.HasPrefix(e.Image, "https://static.tvmaze.com/") {
		t.Fatalf("tvmaze photo %d %s", w.Code, w.Body)
	}
	if w := req(s, "/v1/wikidata/title/movie/404", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("missing %d", w.Code)
	}
	if w := req(s, "/v1/wikidata/award-winners?kind=tv", "synthetic-test-token"); w.Code != 404 || !strings.Contains(w.Body.String(), "list_unavailable") {
		t.Fatalf("empty list %d", w.Code)
	}
	for _, q := range []string{"/v1/wikidata/title/show/1", "/v1/wikidata/title/movie/01", "/v1/wikidata/title/movie/0", "/v1/wikidata/person/x", "/v1/wikidata/artist/abc", "/v1/wikidata/person/1?x=1", "/v1/wikidata/award-winners", "/v1/wikidata/award-winners?kind=book", "/v1/wikidata/other/1"} {
		if w := req(s, q, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%s %d", q, w.Code)
		}
	}
}
