package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/explore/service-top
// Every service's list in one answer, read once from the metadata server
// and kept 12 hours; each list keeps the service's order, split by type;
// TMDB popularity is not ranked; services the app does not know and empty
// lists are left out.
func TestExploreServiceTop(t *testing.T) {
	t.Parallel()
	a, _, _ := exploreRig(t)
	reads := 0
	fail := false
	a.s.ServiceTop = func(_ context.Context, region string) (metadata.ServiceTop, error) {
		reads++
		if fail {
			return metadata.ServiceTop{}, errors.New("down")
		}
		return metadata.ServiceTop{Region: region, Attribution: "Streaming data by Streaming Availability (Movie of the Night)", Services: []metadata.ServiceTopList{
			{Service: "netflix", Name: "Netflix", Source: "streaming-availability", Items: []metadata.ServiceTopItem{{Rank: 1, TMDBID: 701, Kind: "tv", Title: "Show A"}, {Rank: 2, TMDBID: 702, Kind: "movie", Title: "Film B", PosterPath: "/b.jpg"}, {Rank: 3, TMDBID: 703, Kind: "tv", Title: "Show C"}}},
			{Service: "peacock", Name: "Peacock", Source: "tmdb-popular", Items: []metadata.ServiceTopItem{{Rank: 1, TMDBID: 704, Kind: "movie", Title: "Film D"}}},
			{Service: "starz", Name: "Starz", Source: "streaming-availability", Items: []metadata.ServiceTopItem{{Rank: 1, TMDBID: 706, Kind: "movie", Title: "Film E"}}},
			{Service: "tubi", Name: "Tubi", Source: "tmdb-popular", Items: []metadata.ServiceTopItem{{Rank: 1, TMDBID: 707, Kind: "movie", Title: "Film F"}}},
			{Service: "justwatch", Name: "JustWatch TV", Source: "tmdb-popular", Items: []metadata.ServiceTopItem{{Rank: 1, TMDBID: 705, Kind: "movie", Title: "Junk"}}},
		}}, nil
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	base := srv.URL + "/api/v1/explore/service-top"

	all := getJSON(t, base+"?type=all&region=US", 200)
	svcs := all["services"].([]any)
	if all["attribution"] == "" || len(svcs) != 4 {
		t.Fatalf("all %v", all)
	}
	nf := svcs[0].(map[string]any)
	items := nf["items"].([]any)
	if nf["service"] != float64(8) || nf["ranked"] != true || nf["source"] != "streaming-availability" || len(items) != 3 || items[0].(map[string]any)["tmdb_id"] != "701" || items[1].(map[string]any)["rank"] != float64(2) {
		t.Fatalf("netflix %v", nf)
	}
	pc := svcs[1].(map[string]any)
	if pc["service"] != float64(386) || pc["ranked"] != false {
		t.Fatalf("peacock %v", pc)
	}
	if sz, tb := svcs[2].(map[string]any), svcs[3].(map[string]any); sz["service"] != float64(43) || sz["ranked"] != true || tb["service"] != float64(73) || tb["ranked"] != false {
		t.Fatalf("starz %v tubi %v", sz, tb)
	}

	// Shows only: the service's order kept, numbered again.
	shows := getJSON(t, base+"?type=shows&region=US", 200)["services"].([]any)
	got := shows[0].(map[string]any)["items"].([]any)
	if len(shows) != 1 || len(got) != 2 || got[1].(map[string]any)["tmdb_id"] != "703" || got[1].(map[string]any)["rank"] != float64(2) {
		t.Fatalf("shows %v", shows)
	}
	if reads != 1 {
		t.Fatalf("metadata server read %d times, want once", reads)
	}
	for _, bad := range []string{"?type=all&region=usa", "?type=x&region=US", "?type=all"} {
		getJSON(t, base+bad, 400)
	}
	// A failure with no good copy is a 502.
	fail = true
	getJSON(t, base+"?type=all&region=GB", 502)
}
