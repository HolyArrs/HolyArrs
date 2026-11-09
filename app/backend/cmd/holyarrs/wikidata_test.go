package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

func TestWikiFactsBadgesBioAndFallback(t *testing.T) {
	t.Parallel()
	asked := 0
	srv := httptest.NewServer(buildHandlers(services{Version: "test", ImageBase: "https://api.example.test/v1/img",
		WikiTitle: func(_ context.Context, kind, id string) (metadata.WikiEntity, error) {
			asked++
			if id == "3" {
				return metadata.WikiEntity{}, metadata.ErrServiceUnavailable
			}
			if id == "9" {
				return metadata.WikiEntity{}, metadata.ErrNotFound
			}
			return metadata.WikiEntity{Tallies: []metadata.WikiTally{{Family: "golden_globe", Nominations: 2}, {Family: "oscar", Wins: 1}, {Family: "emmy", Wins: 3}},
				Awards:  []metadata.WikiAward{{Name: "Academy Award for Best Picture", Family: "oscar", Won: true, Year: 1995}},
				Summary: &metadata.WikiSummary{Extract: "x", URL: "https://en.wikipedia.org/wiki/X"}}, nil
		},
		WikiPerson: func(_ context.Context, id, _ string) (metadata.WikiEntity, error) {
			// The server's stored photo of a Wikimedia picture comes through the artwork cache.
			return metadata.WikiEntity{Died: "2020-01-02", Image: "https://api.example.test/v1/img/person/" + id + "/500.webp?v=3f",
				Summary: &metadata.WikiSummary{Extract: "An actor.", URL: "https://en.wikipedia.org/wiki/A", License: "CC BY-SA 4.0"}}, nil
		}}).Handler())
	defer srv.Close()
	var v metadata.WikiFacts
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/wiki/movie/1", "", 200), &v)
	if len(v.Badges) != 3 || v.Badges[0] != "Oscar winner" || v.Badges[1] != "3 Emmys" || v.Badges[2] != "Golden Globe nominee" || len(v.Awards) != 1 || v.Bio != nil {
		t.Fatalf("title %+v", v)
	}
	do(t, "GET", srv.URL+"/api/v1/wiki/movie/1", "", 200)
	if asked != 1 {
		t.Fatalf("not kept: %d asks", asked)
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/wiki/person/31", "", 200), &v)
	if v.Bio == nil || v.Bio.Extract != "An actor." || v.Died != "2020-01-02" || v.Image != "/api/v1/artwork/img/person/31/500.webp?v=3f" {
		t.Fatalf("person %+v", v)
	}
	// A server hiccup answers empty and is asked again; not found is kept.
	for range 2 {
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/wiki/show/3", "", 200), &v)
		do(t, "GET", srv.URL+"/api/v1/wiki/show/9", "", 200)
	}
	if len(v.Badges) != 0 || asked != 1+2+1 {
		t.Fatalf("fallback %+v asks %d", v, asked)
	}
	for _, p := range []string{"/api/v1/wiki/movie/01", "/api/v1/wiki/album/1", "/api/v1/wiki/person/x"} {
		do(t, "GET", srv.URL+p, "", 400)
	}
}

// The Award winners collection keeps only titles on the server's
// award-winner list, from lists spread over the decades.
func TestAwardWinnersCollection(t *testing.T) {
	t.Parallel()
	a, _, f := exploreRig(t)
	a.s.AwardWinners = func(_ context.Context, kind string) ([]metadata.AwardWinner, error) {
		var out []metadata.AwardWinner
		for id := int64(1000); id < 1900; id += 2 {
			out = append(out, metadata.AwardWinner{TMDB: id, Wins: 1})
		}
		return out, nil
	}
	c, ok := engine.CategoryByID("awards")
	if !ok {
		t.Fatal("no awards category")
	}
	src := discover.ExploreSource{E: &a.explore, Lister: f.provider()}
	items, err := a.disc.BrowseItems(context.Background(), src, c, "all", false, "")
	if err != nil || len(items) < engine.BrowseMin {
		t.Fatalf("%d items, %v", len(items), err)
	}
	kinds := map[string]bool{}
	for _, it := range items {
		n, _ := strconv.Atoi(it.TMDBID)
		if n%2 != 0 {
			t.Fatalf("%s is not an award winner", it.TMDBID)
		}
		kinds[it.Kind] = true
	}
	if !kinds["movie"] || !kinds["show"] {
		t.Fatalf("kinds %v", kinds)
	}
	if f.asked("vote_count.gte=300") == 0 || f.asked("primary_release_date.gte=1990-01-01") == 0 {
		t.Fatal("decade lists not asked")
	}
	// Without the server's list the keyword lists stay.
	a.s.AwardWinners = nil
	if _, ok, _ := discover.AwardItems(context.Background(), src.List, a.titles.AwardWinners, c, "all", false); ok {
		t.Fatal("award list used without the server")
	}
}
