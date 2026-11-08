package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// UseYouTube sends the reader's calls to the stand-in, and a lookup there
// finds its addresses.
func TestUseYouTubeAsksTheStandIn(t *testing.T) {
	f := &ytFake{modes: map[string]string{"standinKey1": "muxed"}, asked: map[string]int{}, stop: make(chan struct{})}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(f.stop) })
	old := metadata.Reader()
	t.Cleanup(func() { metadata.UseReader(old) })

	metadata.UseYouTube(srv.URL + "/")
	if metadata.Reader().Base != srv.URL+"/" {
		t.Fatalf("reader base = %q", metadata.Reader().Base)
	}
	s, err := metadata.ResolveTrailer(context.Background(), "standinKey1", time.Now(), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "mp4" || !s.Audio || s.Key != "standinKey1" || !metadata.Googlevideo(s.URL) {
		t.Fatalf("stream = %+v", s)
	}
	if f.count("standinKey1") != 1 || f.pages.Load() != 1 {
		t.Fatalf("player calls %d, pages %d; want 1 and 1", f.count("standinKey1"), f.pages.Load())
	}
}
