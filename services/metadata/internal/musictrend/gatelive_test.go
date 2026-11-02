package musictrend

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// Once the charts went live, a later pass that leaves pictures open never
// closes the gate again, across a restart too (LiveFile).
func TestGateStaysOpenOnceLive(t *testing.T) {
	pics := &fakePictures{have: map[string]string{}, missed: map[string]bool{}, answer: map[string]string{}, busy: map[string]int{}}
	chart := []string{id(1)}
	pics.answer["artist|"+id(1)] = "applemusic"
	live := filepath.Join(t.TempDir(), "charts-live")
	p := newPrefetch(pics, chart, nil, nil)
	p.LiveFile = live
	if c := p.Pass(context.Background()); !c.Ready || !p.Ready() {
		t.Fatalf("first pass: %+v", c)
	}
	pics.busy["artist|"+id(2)] = 100
	chart = append(chart, id(2))
	p.Artists = func(context.Context) []string { return chart }
	if c := p.Pass(context.Background()); c.Ready || !p.Ready() {
		t.Fatalf("a later open pass closed the gate: %v %v", c.Ready, p.Ready())
	}
	// After a restart, before any pass.
	p2 := newPrefetch(pics, chart, nil, nil)
	p2.LiveFile = live
	if !p2.Ready() {
		t.Fatal("gate closed after a restart")
	}
	// Without the live mark the first go-live still waits.
	p3 := newPrefetch(pics, chart, nil, nil)
	if p3.Ready() {
		t.Fatal("gate open before the first go-live")
	}
}

// An artist blocked only by a paused source counts as pending (a temporary
// miss) after PendingAfter, so the first go-live is not held by it.
func TestPausedSourceCountsPendingAfterTimeout(t *testing.T) {
	pics := &fakePictures{have: map[string]string{}, missed: map[string]bool{}, answer: map[string]string{}, busy: map[string]int{}}
	chart := []string{id(1), id(2)}
	pics.answer["artist|"+id(1)] = "applemusic"
	pics.busy["artist|"+id(2)] = 1000
	now := time.Unix(1_800_000_000, 0)
	p := newPrefetch(pics, chart, nil, nil)
	p.Now = func() time.Time { return now }
	p.Paused = func(error) bool { return true }
	if c := p.Pass(context.Background()); c.Ready || c.ArtistPending != 0 {
		t.Fatalf("pending before the timeout: %+v", c)
	}
	now = now.Add(31 * time.Minute)
	c := p.Pass(context.Background())
	if !c.Ready || c.ArtistPending != 1 {
		t.Fatalf("paused artist still blocks after the timeout: %+v", c)
	}
	// A failure that is not a pause never counts.
	p2 := newPrefetch(pics, chart, nil, nil)
	p2.Now = func() time.Time { return now }
	p2.Paused = func(error) bool { return false }
	p2.Pass(context.Background())
	now = now.Add(time.Hour)
	if c := p2.Pass(context.Background()); c.Ready {
		t.Fatal("a non-pause failure counted as pending")
	}
}

// An album whose cover is blocked only by a paused source counts as
// pending after PendingAfter too, so it does not hold the first go-live.
func TestPausedAlbumCountsPendingAfterTimeout(t *testing.T) {
	pics := &fakePictures{have: map[string]string{}, missed: map[string]bool{}, answer: map[string]string{}, busy: map[string]int{}}
	chart := []string{id(1)}
	pics.answer["artist|"+id(1)] = "applemusic"
	pics.busy["album|"+id(50)] = 1000
	now := time.Unix(1_800_000_000, 0)
	p := newPrefetch(pics, chart, nil, map[string][]string{id(1): {id(50)}})
	p.Now = func() time.Time { return now }
	p.Paused = func(error) bool { return true }
	if c := p.Pass(context.Background()); c.Ready || c.AlbumPending != 0 {
		t.Fatalf("pending before the timeout: %+v", c)
	}
	now = now.Add(31 * time.Minute)
	if c := p.Pass(context.Background()); !c.Ready || c.AlbumPending != 1 {
		t.Fatalf("paused album still blocks after the timeout: %+v", c)
	}
}
