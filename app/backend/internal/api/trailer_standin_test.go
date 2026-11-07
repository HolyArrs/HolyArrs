package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// UseYouTube sends the reader's calls to the stand-in, and a lookup there
// finds its addresses.
func TestUseYouTubeAsksTheStandIn(t *testing.T) {
	f := &ytFake{modes: map[string]string{"standinKey1": "muxed"}, asked: map[string]int{}, stop: make(chan struct{})}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(f.stop) })
	old := trailerReader
	t.Cleanup(func() { trailerReader = old })

	UseYouTube(srv.URL + "/")
	if trailerReader.Base != srv.URL+"/" {
		t.Fatalf("reader base = %q", trailerReader.Base)
	}
	s, err := resolveTrailer(context.Background(), "standinKey1", time.Now(), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "mp4" || !s.Audio || s.Key != "standinKey1" || !googlevideo(s.URL) {
		t.Fatalf("stream = %+v", s)
	}
	if f.count("standinKey1") != 1 || f.pages.Load() != 1 {
		t.Fatalf("player calls %d, pages %d; want 1 and 1", f.count("standinKey1"), f.pages.Load())
	}
}

// TrailerPassTiming sets the first wait and the gap between passes; zero
// or negative keeps the one in use.
func TestTrailerPassTiming(t *testing.T) {
	first, every := trailerPassFirst, trailerPassEvery
	t.Cleanup(func() { trailerPassFirst, trailerPassEvery = first, every })
	if first != 3*time.Minute || every != 5*time.Minute {
		t.Fatalf("defaults = %v and %v, want 3m and 5m", first, every)
	}

	TrailerPassTiming(2*time.Second, 4*time.Second)
	if trailerPassFirst != 2*time.Second || trailerPassEvery != 4*time.Second {
		t.Fatalf("after set: %v and %v", trailerPassFirst, trailerPassEvery)
	}
	TrailerPassTiming(0, -time.Second)
	if trailerPassFirst != 2*time.Second || trailerPassEvery != 4*time.Second {
		t.Fatalf("zero and negative changed them: %v and %v", trailerPassFirst, trailerPassEvery)
	}
	TrailerPassTiming(0, 6*time.Second)
	if trailerPassFirst != 2*time.Second || trailerPassEvery != 6*time.Second {
		t.Fatalf("every alone: %v and %v", trailerPassFirst, trailerPassEvery)
	}

}
