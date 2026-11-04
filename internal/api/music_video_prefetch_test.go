package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// route: GET /api/v1/trailer/batch
// An artist named in the in-view batch has its music video's stream looked
// up ahead of time, like a trailer, within the same ahead-of-time budget;
// the artist page then answers from that warm cache without running
// a lookup again.
func TestArtistVideoPrefetch(t *testing.T) {
	yt := fakeReader(t, "heat", "dQw4w9WgXcQ")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ", MostReplayed: 42}}, nil
	})
	base := u[:strings.Index(u, "/api/v1/")]
	id := "artist:" + apiArtist
	var b trailerBatchView
	if err := json.Unmarshal(do(t, "GET", base+"/api/v1/trailer/batch?ids="+id, "", 200), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Pending) != 1 || b.Pending[0] != id {
		t.Fatalf("first batch %+v", b)
	}
	deadline := time.Now().Add(10 * time.Second)
	for b.Videos[id].ProxyURL == "" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		b = trailerBatchView{}
		if err := json.Unmarshal(do(t, "GET", base+"/api/v1/trailer/batch?ids="+id, "", 200), &b); err != nil {
			t.Fatal(err)
		}
	}
	v := b.Videos[id]
	if v.Key != "dQw4w9WgXcQ" || v.Start != 42 || !strings.HasSuffix(v.ProxyURL, "/manifest.m3u8") || len(b.Pending) != 0 {
		t.Fatalf("batch %+v", b)
	}
	before := yt.runs.Load()
	var got ArtistVideoView
	_ = json.Unmarshal(do(t, "GET", u, "", 200), &got)
	if got.Key != "dQw4w9WgXcQ" || got.ProxyURL == "" || yt.runs.Load() != before {
		t.Fatalf("page video %+v, lookups %d -> %d", got, before, yt.runs.Load())
	}
	// Not a MusicBrainz id.
	do(t, "GET", base+"/api/v1/trailer/batch?ids=artist:nope", "", 400)
}
