package shows

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A show switched to TVMaze keeps the score TMDB gave it; one that never had a
// score is still marked as looked up, so the back-fill does not pick it again.
func TestShowScoreKeptOffTMDB(t *testing.T) {
	var ti mediafiles.Title
	applyShow(&ti, metadata.Show{Source: "tmdb", Name: "Mashle", Score: 8.4, Votes: 12000})
	if ti.Score == nil || *ti.Score != 8.4 || ti.Votes != 12000 {
		t.Fatalf("tmdb score %v %d", ti.Score, ti.Votes)
	}
	applyShow(&ti, metadata.Show{Source: "tvmaze", Name: "Mashle"})
	if ti.Score == nil || *ti.Score != 8.4 || ti.Votes != 12000 {
		t.Fatalf("score lost on tvmaze %v %d", ti.Score, ti.Votes)
	}
	var fresh mediafiles.Title
	applyShow(&fresh, metadata.Show{Source: "tvmaze", Name: "New"})
	if fresh.Score == nil || *fresh.Score != 0 {
		t.Fatalf("tvmaze show without a score: %v", fresh.Score)
	}
}
