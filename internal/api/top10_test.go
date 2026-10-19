package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

func TestTop10JoinsLibraryAndKeepsRanks(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	var calls atomic.Int32
	var asked []string
	top := func(_ context.Context, kind, region, lang string) (metadata.Top10, error) {
		calls.Add(1)
		asked = append(asked, kind+"|"+region+"|"+lang)
		if region == "ZZ" {
			return metadata.Top10{}, metadata.ErrNotFound
		}
		return metadata.Top10{Week: "2026-10-04", Regions: []string{"global", "US"}, Items: []metadata.Top10Item{
			{Rank: 1, TMDBID: 11, Title: "UNABOMBER", PosterPath: "/u.jpg", Views: 5, Weeks: 2},
			{Rank: 2, Title: "Unknown"},
			{Rank: 3, TMDBID: 13, Title: "Never"},
			{Rank: 4, TMDBID: 12, Title: "Asked", Season: "Asked: Season 2"},
			{Rank: 5, TMDBID: 12, Title: "Asked", Season: "Asked: Season 1"},
		}}, nil
	}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Top10: top, Version: "test"}).Handler())
	defer srv.Close()
	get := func(q string, want int) map[string]any {
		resp, err := http.Get(srv.URL + "/api/v1/top10" + q)
		if err != nil || resp.StatusCode != want {
			t.Fatalf("%s: %v %d", q, err, resp.StatusCode)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	a := get("?kind=movie", 200)
	items := a["items"].([]any)
	if a["week"] != "2026-10-04" || a["region"] != "global" || len(a["regions"].([]any)) != 2 || len(items) != 2 {
		t.Fatalf("answer %+v", a)
	}
	first, second := items[0].(map[string]any), items[1].(map[string]any)
	if first["rank"] != 1.0 || first["tmdb_id"] != "11" || first["in_library"] != true || first["poster"] != metadata.ImageBase+"/w342/u.jpg" || second["rank"] != 4.0 || second["ask"] == nil {
		t.Fatalf("items %+v", items)
	}
	get("?kind=movie", 200)
	if calls.Load() != 1 {
		t.Fatal("list not cached")
	}
	get("?kind=show&region=us", 200)
	if asked[1] != "tv|US|" {
		t.Fatalf("asked %v", asked)
	}
	get("?kind=movie&region=ZZ", 404)
	get("?kind=album", 400)
	get("?kind=movie&lang=fr", 400)
	srv2 := httptest.NewServer(New(Server{Store: s, Version: "test"}).Handler())
	defer srv2.Close()
	if resp, _ := http.Get(srv2.URL + "/api/v1/top10?kind=movie"); resp.StatusCode != 404 {
		t.Fatalf("without a server %d", resp.StatusCode)
	}
}
