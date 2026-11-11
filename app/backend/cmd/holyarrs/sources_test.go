package main

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// An install asks the metadata server for show details; TVmaze is asked
// directly only when a stand-in address is named (tests).
func TestShowSourcesHostedUnlessStandIn(t *testing.T) {
	cases := []struct {
		url, direct string
		wantDirect  bool
	}{
		{"", "", false},
		{"", "1", false},
		{"off", "", false},
		{" off ", "1", false},
		{"   ", "", false},
		{"http://127.0.0.1:9", "", true},
		{"http://127.0.0.1:9", "1", true},
	}
	for _, c := range cases {
		t.Setenv("HOLYARRS_TVMAZE_URL", c.url)
		t.Setenv("HOLYARRS_DIRECT_SOURCES", c.direct)
		src := showSources(nil, &metadata.Hosted{Service: &metadata.Service{}})
		_, isDirect := src["tvmaze"].(*metadata.TVMaze)
		if isDirect != c.wantDirect {
			t.Errorf("TVMAZE_URL=%q DIRECT_SOURCES=%q: direct TVmaze = %v, want %v", c.url, c.direct, isDirect, c.wantDirect)
		}
		if _, bare := src["tmdb"].(*metadata.TMDB); bare {
			t.Errorf("TVMAZE_URL=%q: the TMDB source skips anime release numbers", c.url)
		}
		if src["tmdb"] == nil || src["tvdb"] != nil {
			t.Errorf("TVMAZE_URL=%q: want tmdb and no tvdb source", c.url)
		}
	}
	t.Setenv("X_STAND_IN", " off ")
	if standIn("X_STAND_IN") {
		t.Error(`standIn(" off ") is true`)
	}
	t.Setenv("X_STAND_IN", "http://x")
	if !standIn("X_STAND_IN") {
		t.Error("an address is not a stand-in")
	}
}
