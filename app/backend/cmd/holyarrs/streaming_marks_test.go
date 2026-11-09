package main

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestStreamingMarks(t *testing.T) {
	t.Parallel()
	a := &handlers{}
	a.serviceTop.Put("US", metadata.ServiceTop{Services: []metadata.ServiceTopList{
		{Service: "netflix", Name: "Netflix", Source: "streaming-availability", Items: []metadata.ServiceTopItem{{Rank: 3, TMDBID: 7, Kind: "tv"}}},
		{Service: "peacock", Name: "Peacock", Source: "tmdb-popular", Items: []metadata.ServiceTopItem{{TMDBID: 9, Kind: "movie"}, {TMDBID: 7, Kind: "tv"}}},
	}})
	m := a.serviceTop.Marks("US", []int{386, 8})
	if got := m["show:7"]; got.Service != 386 || got.Rank != 0 {
		t.Fatalf("first service in the member's order wins, unranked: %+v", got)
	}
	if got := m["movie:9"]; got.Name != "Peacock" {
		t.Fatalf("movie %+v", got)
	}
	if got := a.serviceTop.Marks("US", []int{8})["show:7"]; got.Rank != 3 || got.Name != "Netflix" {
		t.Fatalf("ranked %+v", got)
	}
	if a.serviceTop.Marks("GB", []int{8}) != nil || a.serviceTop.Marks("US", nil) != nil {
		t.Fatal("no kept list or no services: no marks")
	}
}
