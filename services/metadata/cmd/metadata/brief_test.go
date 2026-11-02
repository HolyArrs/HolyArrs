package main

import (
	"context"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
)

// A picture's artist subject comes from the slim copy's brief read (no
// full answer built) when the copy has the artist.
func TestArtistSubjectFromBrief(t *testing.T) {
	sp := &speed{brief: func(_ context.Context, mbid string) (mbread.Brief, bool) {
		return mbread.Brief{Name: "Test Band", Country: "GB", Links: []string{"https://open.spotify.com/artist/x"}}, mbid == "a"
	}}
	s := sp.artistSubject(context.Background(), "a")
	if !s.Known || s.Name != "Test Band" || s.Country != "GB" || len(s.Links) != 1 {
		t.Fatalf("subject %+v", s)
	}
	if sp.artistSubject(context.Background(), "b").Known {
		t.Fatal("unknown artist known without a stored answer")
	}
}
