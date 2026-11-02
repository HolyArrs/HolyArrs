package api

import (
	"context"
	"testing"
)

func TestWarmMusicArtistFetchesOnceThenKeeps(t *testing.T) {
	s, _, calls := setup(Options{Burst: 100})
	const id = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	got, err := s.WarmMusicArtist(context.Background(), id)
	if err != nil || !got || calls.Load() != 1 {
		t.Fatalf("first warm = %v %v, calls %d", got, err, calls.Load())
	}
	got, err = s.WarmMusicArtist(context.Background(), id)
	if err != nil || got || calls.Load() != 1 {
		t.Fatalf("second warm = %v %v, calls %d", got, err, calls.Load())
	}
	// The page's own request is a cache hit (same key).
	if w := req(s, "/v1/music/musicbrainz/artist/"+id, "synthetic-test-token"); w.Code != 200 || w.Header().Get("X-Metadata-Cache") != "hit" || calls.Load() != 1 {
		t.Fatalf("page lookup = %d %q, calls %d", w.Code, w.Header().Get("X-Metadata-Cache"), calls.Load())
	}
	if _, err := s.WarmMusicArtist(context.Background(), "not-an-id"); err == nil {
		t.Fatal("bad id warmed")
	}
}
