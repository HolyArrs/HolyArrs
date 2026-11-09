package music

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/music/albums/{mbid}
// "More like this" keeps an empty answer: the next visit answers at once
// from the store and never asks the similar list again.
func TestAlbumNearKeepsEmptyAnswer(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	var asked atomic.Int32
	r.api.s.MusicSimilar = func(_ context.Context, _ string) ([]metadata.SimilarArtist, error) {
		asked.Add(1)
		return []metadata.SimilarArtist{{MBID: "a0000000-0000-0000-0000-0000000000bb", Name: "Other"}}, nil
	}
	// The artist data never answers in time: the list stays empty.
	r.api.s.MusicArtistData = func(ctx context.Context, _ []string) ([]metadata.MusicArtistData, error) {
		asked.Add(1)
		select {
		case <-time.After(1200 * time.Millisecond):
		case <-ctx.Done():
		}
		return nil, metadata.ErrServiceUnavailable
	}
	path := "/api/v1/music/albums/" + apiAlbum + "?part=near&artist=" + apiArtist
	var near struct {
		Near []PageCredit `json:"near"`
	}
	if err := json.Unmarshal(r.ana.call("GET", path, "", 200), &near); err != nil || near.Near == nil || len(near.Near) != 0 {
		t.Fatalf("first %+v %v", near, err)
	}
	first := asked.Load()
	for i := 0; i < 3; i++ {
		start := time.Now()
		if err := json.Unmarshal(r.ana.call("GET", path, "", 200), &near); err != nil || near.Near == nil {
			t.Fatalf("again %+v %v", near, err)
		}
		if d := time.Since(start); d > 300*time.Millisecond {
			t.Fatalf("a kept empty answer took %v", d)
		}
	}
	if asked.Load() != first {
		t.Fatalf("similar asked %d times after the first visit (%d)", asked.Load(), first)
	}
}
