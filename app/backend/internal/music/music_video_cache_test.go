package music

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Cache audit 23 October 2026, finding 2: a pending, failed or empty video
// answer is not kept an hour; a non-empty list is kept on disk and
// re-checked daily in the background.

func videoAPI(t *testing.T, st *database.Store, vids func(context.Context, string) ([]metadata.ArtistVideo, error)) *API {
	t.Helper()
	if st == nil {
		var err error
		st, err = database.Open(t.TempDir(), database.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
	}
	return New(Server{Store: st, ArtistVideos: vids})
}

func TestVideosPendingNotKept(t *testing.T) {
	var calls atomic.Int32
	a := videoAPI(t, nil, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		if calls.Add(1) == 1 {
			return nil, metadata.ErrServicePending
		}
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}}, nil
	})
	if l := a.videosOf(context.Background(), apiArtist); len(l) != 0 {
		t.Fatalf("pending %v", l)
	}
	if l := a.videosOf(context.Background(), apiArtist); len(l) != 1 {
		t.Fatalf("pending was kept: %v (calls %d)", l, calls.Load())
	}
}

func TestVideosEmptyKeptMinutes(t *testing.T) {
	var calls atomic.Int32
	a := videoAPI(t, nil, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		if calls.Add(1) == 1 {
			return nil, nil
		}
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}}, nil
	})
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	a.artistVideos.now = func() time.Time { return now }
	_ = a.videosOf(context.Background(), apiArtist)
	_ = a.videosOf(context.Background(), apiArtist)
	if calls.Load() != 1 {
		t.Fatalf("empty not kept briefly: %d", calls.Load())
	}
	now = now.Add(6 * time.Minute)
	if l := a.videosOf(context.Background(), apiArtist); len(l) != 1 {
		t.Fatalf("empty kept past 5 minutes: %v", l)
	}
}

func TestVideosOnDiskAndRecheckedDaily(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a := videoAPI(t, st, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ"}}, nil
	})
	if l := a.videosOf(context.Background(), apiArtist); len(l) != 1 {
		t.Fatalf("first %v", l)
	}
	// A restart: the server is down, the disk copy answers.
	var calls atomic.Int32
	refreshed := make(chan struct{}, 1)
	b := videoAPI(t, st, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		calls.Add(1)
		refreshed <- struct{}{}
		return []metadata.ArtistVideo{{ID: "AAAAAAAAAAA"}}, nil
	})
	if l := b.videosOf(context.Background(), apiArtist); len(l) != 1 || l[0].ID != "dQw4w9WgXcQ" || calls.Load() != 0 {
		t.Fatalf("disk copy %v calls %d", l, calls.Load())
	}
	// A day later the page still answers at once from the kept list and a
	// background check replaces it.
	b.artistVideos.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	if l := b.videosOf(context.Background(), apiArtist); len(l) != 1 || l[0].ID != "dQw4w9WgXcQ" {
		t.Fatalf("stale answer %v", l)
	}
	select {
	case <-refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("no background re-check")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if l := b.videosOf(context.Background(), apiArtist); len(l) == 1 && l[0].ID == "AAAAAAAAAAA" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("background re-check not applied")
}

// A video that will not play is dropped and another one is picked.
func TestFailedVideoDroppedAndAnotherPicked(t *testing.T) {
	fakeReader(t, "plain", "dQw4w9WgXcQ")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "badbadbad11"}, {ID: "badbadbad22"}, {ID: "dQw4w9WgXcQ"}}, nil
	})
	for i := 0; i < 4; i++ {
		var v ArtistVideoView
		_ = json.Unmarshal(videoReady(t, u), &v)
		if v.Key != "dQw4w9WgXcQ" {
			t.Fatalf("picked %+v", v)
		}
	}
}

// Finding 15: the cap drops the oldest entries, not the whole map.
func TestVideoCacheEvictsOldest(t *testing.T) {
	c := &artistVideoCache{}
	base := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	for i := 0; i < artistVideosMax+1; i++ {
		c.putLocked(string(rune('a'+i%26))+time.Duration(i).String(), artistVideoList{at: base.Add(time.Duration(i) * time.Second)})
	}
	if len(c.m) > artistVideosMax || len(c.m) < artistVideosMax*8/10 {
		t.Fatalf("size %d", len(c.m))
	}
	if _, ok := c.m["a0s"]; ok {
		t.Fatal("oldest kept")
	}
}
