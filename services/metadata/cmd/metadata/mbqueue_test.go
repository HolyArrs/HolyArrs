package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
)

// The MusicBrainz queue runs two jobs at once (one kept for P0 and P1) and
// pauses as a whole on a connection failure; musictags is idle only while
// nothing at P0 to P2 is ready.
func TestMusicBrainzQueueWorkersAndIdle(t *testing.T) {
	sp := openTestSpeed(t, &api.Options{})
	ctx := context.Background()
	if st := sp.mbq.Stats(ctx); st.Workers != 2 {
		t.Fatalf("workers %d", st.Workers)
	}
	if !sp.mbIdle(ctx) {
		t.Fatal("empty queue not idle")
	}
	sp.mbq.Enqueue(ctx, "mb:artist:11111111-1111-1111-1111-111111111111", fetchq.P3Refresh, false)
	if !sp.mbIdle(ctx) {
		t.Fatal("a P3 job made it busy")
	}
	sp.mbq.Enqueue(ctx, "mb:artist:22222222-2222-2222-2222-222222222222", fetchq.P2Shared, false)
	if sp.mbIdle(ctx) {
		t.Fatal("a P2 job left it idle")
	}
}

type countRT struct{ n atomic.Int64 }

func (c *countRT) RoundTrip(*http.Request) (*http.Response, error) {
	c.n.Add(1)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}

// The catalog's MetaBrainz calls wait while the queue is busy, go once it
// is idle, and go anyway after idleMax; other hosts never wait.
func TestIdleTransport(t *testing.T) {
	poll, mx := idlePoll, idleMax
	idlePoll, idleMax = 5*time.Millisecond, 300*time.Millisecond
	defer func() { idlePoll, idleMax = poll, mx }()
	var busy atomic.Bool
	busy.Store(true)
	next := &countRT{}
	var held atomic.Int64
	tr := idleTransport{next: next, idle: func(context.Context) bool { return !busy.Load() }, held: &held}
	get := func(u string) time.Duration {
		r, _ := http.NewRequest("GET", u, nil)
		start := time.Now()
		if _, err := tr.RoundTrip(r); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	if d := get("https://api.deezer.com/x"); d > 50*time.Millisecond {
		t.Fatalf("other host waited %v", d)
	}
	go func() { time.Sleep(60 * time.Millisecond); busy.Store(false) }()
	if d := get("https://musicbrainz.org/ws/2/x"); d < 50*time.Millisecond || d > 250*time.Millisecond {
		t.Fatalf("musicbrainz call went after %v", d)
	}
	busy.Store(true)
	if d := get("https://api.listenbrainz.org/1/x"); d < 290*time.Millisecond {
		t.Fatalf("held call did not wait for idleMax: %v", d)
	}
	if next.n.Load() != 3 || held.Load() == 0 {
		t.Fatalf("calls %d held %d", next.n.Load(), held.Load())
	}
}
