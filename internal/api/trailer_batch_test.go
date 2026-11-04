package api

import (
	"encoding/json"
	"testing"
)

// route: GET /api/v1/trailer/batch
func TestTrailerBatch(t *testing.T) {
	yt := fakeReader(t, "ok")
	srv, a := trailerRig(t)
	var b trailerBatchView
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11,movie:11", "", 200), &b); err != nil {
		t.Fatal(err)
	}
	// Not looked up yet: left out, nothing pending (the page does not ask
	// again), and no lookup starts.
	if len(b.Streams) != 0 || len(b.Pending) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("first batch %+v, lookups %d", b, yt.runs.Load())
	}
	// Once the trailer pass kept it, a batch has it.
	if err := lookUp(t, a, "movie", "11"); err != nil {
		t.Fatal(err)
	}
	b = trailerBatchView{}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11", "", 200), &b); err != nil {
		t.Fatal(err)
	}
	s, ok := b.Streams["movie:11"]
	if !ok || s.Type != "hls" || s.Key != "trailerKey1" || s.ProxyURL == "" || len(b.Pending) != 0 {
		t.Fatalf("batch %+v", b)
	}
	// The single route answers from the cache.
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
