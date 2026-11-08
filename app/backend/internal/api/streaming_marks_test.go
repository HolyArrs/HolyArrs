package api

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestStreamingMarks(t *testing.T) {
	t.Parallel()
	a := &API{}
	a.serviceTop.regions = map[string]serviceTopEntry{"US": {at: time.Now(), list: metadata.ServiceTop{Services: []metadata.ServiceTopList{
		{Service: "netflix", Name: "Netflix", Source: "streaming-availability", Items: []metadata.ServiceTopItem{{Rank: 3, TMDBID: 7, Kind: "tv"}}},
		{Service: "peacock", Name: "Peacock", Source: "tmdb-popular", Items: []metadata.ServiceTopItem{{TMDBID: 9, Kind: "movie"}, {TMDBID: 7, Kind: "tv"}}},
	}}}}
	m := a.streamingMarks("US", []int{386, 8})
	if got := m["show:7"]; got.Service != 386 || got.Rank != 0 {
		t.Fatalf("first service in the member's order wins, unranked: %+v", got)
	}
	if got := m["movie:9"]; got.Name != "Peacock" {
		t.Fatalf("movie %+v", got)
	}
	if got := a.streamingMarks("US", []int{8})["show:7"]; got.Rank != 3 || got.Name != "Netflix" {
		t.Fatalf("ranked %+v", got)
	}
	if a.streamingMarks("GB", []int{8}) != nil || a.streamingMarks("US", nil) != nil {
		t.Fatal("no kept list or no services: no marks")
	}
}
