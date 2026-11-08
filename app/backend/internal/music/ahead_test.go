package music

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type aheadArtists struct {
	mu       sync.Mutex
	artists  []string
	albums   []string
	err      error
	marked   bool
	unmarked bool
}

func (f *aheadArtists) SearchArtists(context.Context, string) ([]metadata.Artist, error) {
	return nil, nil
}

func (f *aheadArtists) Artist(ctx context.Context, mbid string) (metadata.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.artists = append(f.artists, mbid)
	if metadata.IsAhead(ctx) {
		f.marked = true
	} else {
		f.unmarked = true
	}
	return metadata.Artist{MBID: mbid, Name: "A"}, f.err
}

func (f *aheadArtists) Pressings(ctx context.Context, mbid string) ([]metadata.Pressing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.albums = append(f.albums, mbid)
	return nil, f.err
}

func (f *aheadArtists) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.artists) + len(f.albums)
}

const (
	aheadArtistA = "5b11f4ce-a62d-471e-81fc-a69a8278c7da"
	aheadAlbumB  = "1b022e01-4da6-387b-8658-8678046e4cef"
)

func fastAhead(t *testing.T) {
	g, ag, b := aheadGap, aheadArtGap, aheadBackoff
	aheadGap, aheadArtGap, aheadBackoff = 0, 0, time.Millisecond
	t.Cleanup(func() { aheadGap, aheadArtGap, aheadBackoff = g, ag, b })
}

func TestFetchAheadOnceThenRebuildMakesNoFetch(t *testing.T) {
	fastAhead(t)
	f := &aheadArtists{}
	a := newTestAPI(Server{Version: "test", Library: &library.Service{Artists: f}})
	items := aheadOf([][2]string{{"artist", aheadArtistA}, {"album", aheadAlbumB}, {"artist", aheadArtistA}, {"album", "not-an-id"}})
	if len(items) != 2 {
		t.Fatalf("items = %v", items)
	}
	a.fetchAhead(context.Background(), items)
	if len(f.artists) != 1 || len(f.albums) != 1 || !f.marked || f.unmarked {
		t.Fatalf("first pass: artists %v albums %v marked %v", f.artists, f.albums, f.marked)
	}
	// A rebuild picks the same items: re-ranked from kept data, no fetch.
	a.fetchAhead(context.Background(), items)
	if f.calls() != 2 {
		t.Fatalf("a rebuild must not fetch unchanged items again: %d calls", f.calls())
	}
}

func TestFetchAheadFollowsSwitch(t *testing.T) {
	fastAhead(t)
	f := &aheadArtists{}
	var reads int
	sw := metadata.Control{Prefetch: "off"}
	a := newTestAPI(Server{Version: "test", Library: &library.Service{Artists: f}, MusicControl: func(context.Context) (metadata.Control, error) {
		reads++
		return sw, nil
	}})
	a.fetchAhead(context.Background(), aheadOf([][2]string{{"artist", aheadArtistA}}))
	if f.calls() != 0 {
		t.Fatalf("off must stop fetch-ahead, got %d calls", f.calls())
	}
	// Read again only after 15 minutes.
	a.musicSwitch(context.Background(), time.Now())
	if reads != 1 {
		t.Fatalf("switch read %d times within 15 min", reads)
	}
	if c := a.musicSwitch(context.Background(), time.Now().Add(16*time.Minute)); reads != 2 || c.Prefetch != "off" {
		t.Fatalf("reads %d, %+v", reads, c)
	}
}

func TestMusicSwitchOldServerAndStale(t *testing.T) {
	old := newTestAPI(Server{Version: "test", MusicControl: func(context.Context) (metadata.Control, error) { return metadata.Control{}, metadata.ErrNotFound }})
	if c := old.musicSwitch(context.Background(), time.Now()); c.Prefetch != "on" {
		t.Fatalf("an older server without the switch is on: %+v", c)
	}
	var fail bool
	a := newTestAPI(Server{Version: "test", MusicControl: func(context.Context) (metadata.Control, error) {
		if fail {
			return metadata.Control{}, metadata.ErrServiceUnavailable
		}
		return metadata.Control{Prefetch: "on"}, nil
	}})
	now := time.Now()
	a.musicSwitch(context.Background(), now)
	fail = true
	if c := a.musicSwitch(context.Background(), now.Add(time.Hour)); c.Prefetch != "on" {
		t.Fatalf("a failed read keeps the last value: %+v", c)
	}
	if c := a.musicSwitch(context.Background(), now.Add(25*time.Hour)); c.Prefetch != "slow" {
		t.Fatalf("a day without a good read acts as slow: %+v", c)
	}
}

func TestFetchAheadBacksOffAndStops(t *testing.T) {
	fastAhead(t)
	f := &aheadArtists{err: metadata.ErrServiceBusy}
	a := newTestAPI(Server{Version: "test", Library: &library.Service{Artists: f}})
	var pairs [][2]string
	for _, id := range []string{aheadArtistA, "6b11f4ce-a62d-471e-81fc-a69a8278c7da", "7b11f4ce-a62d-471e-81fc-a69a8278c7da", "8b11f4ce-a62d-471e-81fc-a69a8278c7da", "9b11f4ce-a62d-471e-81fc-a69a8278c7da"} {
		pairs = append(pairs, [2]string{"artist", id})
	}
	a.fetchAhead(context.Background(), aheadOf(pairs))
	if f.calls() != aheadFails {
		t.Fatalf("busy answers stop the run after %d, got %d calls", aheadFails, f.calls())
	}
	// Nothing was kept: the next prebuild tries again.
	f.err = nil
	a.fetchAhead(context.Background(), aheadOf(pairs[:1]))
	if f.calls() != aheadFails+1 {
		t.Fatalf("a busy item is not marked kept")
	}
}

func TestMusicPrebuildGapJitter(t *testing.T) {
	lo, hi := musicPrebuildEvery/2, musicPrebuildEvery*3/2
	seen := map[time.Duration]bool{}
	for range 200 {
		d := musicPrebuildGap(metadata.Control{Prefetch: "on"})
		if d < lo || d > hi {
			t.Fatalf("gap %v outside %v..%v", d, lo, hi)
		}
		seen[d/time.Minute] = true
		if s := musicPrebuildGap(metadata.Control{Prefetch: "slow"}); s < musicPrebuildEvery {
			t.Fatalf("slow gap %v", s)
		}
		if m := musicPrebuildGap(metadata.Control{Prefetch: "on", PrebuildMinHours: 6}); m < 3*time.Hour || m > 9*time.Hour {
			t.Fatalf("min-hours gap %v", m)
		}
	}
	if len(seen) < 20 {
		t.Fatalf("gaps are not spread: %d distinct minutes", len(seen))
	}
	at := time.Now()
	if spreadAt(time.Hour, at, 0.25) != spreadAt(time.Hour, at, 0.25) {
		t.Fatal("a kept list's spread is fixed")
	}
}

func TestQueueAheadNeedsMusicLoop(t *testing.T) {
	fastAhead(t)
	f := &aheadArtists{}
	a := newTestAPI(Server{Version: "test", Library: &library.Service{Artists: f}})
	a.queueAhead(aheadOf([][2]string{{"artist", aheadArtistA}}))
	time.Sleep(20 * time.Millisecond)
	if f.calls() != 0 {
		t.Fatal("no fetch-ahead while Music's loop is off")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.beginAhead(ctx)
	a.queueAhead(aheadOf([][2]string{{"artist", aheadArtistA}, {"album", aheadAlbumB}}))
	deadline := time.Now().Add(2 * time.Second)
	for f.calls() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if f.calls() != 2 {
		t.Fatalf("queued items fetched: %d", f.calls())
	}
	a.endAhead()
}
