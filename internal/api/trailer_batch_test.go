package api

import (
	"encoding/json"
	"testing"
	"time"
)

// route: GET /api/v1/trailer/batch
func TestTrailerBatch(t *testing.T) {
	yt := fakeReader(t, "ok")
	srv, _ := discoverRig(t, trailerTMDB(t))
	var b trailerBatchView
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11,movie:11", "", 200), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Streams) != 0 || len(b.Pending) != 1 || b.Pending[0] != "movie:11" {
		t.Fatalf("first batch %+v", b)
	}
	// Looked up ahead of time: a later batch has it, without a person's resolve.
	deadline := time.Now().Add(10 * time.Second)
	for len(b.Streams) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		b = trailerBatchView{}
		if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11", "", 200), &b); err != nil {
			t.Fatal(err)
		}
	}
	s, ok := b.Streams["movie:11"]
	if !ok || s.Type != "hls" || s.Key != "trailerKey1" || s.ProxyURL == "" || len(b.Pending) != 0 {
		t.Fatalf("batch %+v", b)
	}
	// The single route now answers from the cache.
	before := yt.runs.Load()
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200)
	if yt.runs.Load() != before {
		t.Fatalf("looked up again")
	}
	do(t, "GET", srv.URL+"/api/v1/trailer/batch", "", 400)
	do(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=artist:1", "", 400)
	do(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:x", "", 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	do(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11", "", 404)
}
