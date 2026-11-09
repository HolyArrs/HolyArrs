package music

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/music/artists/{mbid}?part=similar
// While the metadata server is still getting an artist's similar list
// (202), the part answers incomplete so the page asks again, and the next
// ask reaches the server instead of keeping the miss for an hour.
func TestArtistSimilarPendingAsksAgain(t *testing.T) {
	old := similarPendingAgain
	similarPendingAgain = 0
	defer func() { similarPendingAgain = old }()
	r := newMusicAskRig(t)
	var asked atomic.Int32
	r.api.s.MusicSimilar = func(_ context.Context, _ string) ([]metadata.SimilarArtist, error) {
		if asked.Add(1) == 1 {
			return nil, metadata.ErrServicePending
		}
		return []metadata.SimilarArtist{{MBID: "a0000000-0000-0000-0000-0000000000bb", Name: "Other"}}, nil
	}
	path := "/api/v1/music/artists/" + apiArtist + "?part=similar"
	var got struct {
		Similar    []PageArtist `json:"similar"`
		Incomplete bool         `json:"incomplete"`
	}
	if err := json.Unmarshal(r.ana.call("GET", path, "", 200), &got); err != nil || len(got.Similar) != 0 || !got.Incomplete {
		t.Fatalf("first %+v %v", got, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		got.Incomplete = false
		if err := json.Unmarshal(r.ana.call("GET", path, "", 200), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Similar) == 1 && !got.Incomplete {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("never filled: %+v (asked %d)", got, asked.Load())
		}
		time.Sleep(50 * time.Millisecond)
	}
}
