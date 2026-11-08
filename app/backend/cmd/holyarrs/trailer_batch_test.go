package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// route: GET /api/v1/trailer/batch
func TestTrailerBatch(t *testing.T) {
	yt := fakeReader(t, "ok")
	srv, a := trailerRig(t)
	var b trailerBatchView
	if err := json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11,movie:11", "", 200), &b); err != nil {
		t.Fatal(err)
	}
	// Not looked up yet: left out, nothing pending (the page does not ask
	// again), and no lookup starts.
	if len(b.Streams) != 0 || len(b.Pending) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("first batch %+v, lookups %d", b, yt.runs.Load())
	}
	// ...but noted, once, so the next pass looks it up (cards outside the
	// stored Explore pools get covered this way).
	if got := a.Trailers().Asks(time.Now()); len(got) != 1 || got[0][0] != "movie" || got[0][1] != "11" {
		t.Fatalf("noted %v", got)
	}
	// Once the trailer pass kept it, a batch has it.
	if err := lookUp(t, a, "movie", "11"); err != nil {
		t.Fatal(err)
	}
	b = trailerBatchView{}
	if err := json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11", "", 200), &b); err != nil {
		t.Fatal(err)
	}
	s, ok := b.Streams["movie:11"]
	if !ok || s.Type != "hls" || s.Key != "trailerKey1" || s.ProxyURL == "" || len(b.Pending) != 0 {
		t.Fatalf("batch %+v", b)
	}
	// A kept title is not noted again.
	a.Trailers().DropAsk(a.Trailers().Asks(time.Now())[0][0] + "|11|" + a.Trailers().Asks(time.Now())[0][2])
	trDo(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11", "", 200)
	if got := a.Trailers().Asks(time.Now()); len(got) != 0 {
		t.Fatalf("kept, noted again %v", got)
	}
	// The single route answers from the cache.
	before := yt.runs.Load()
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200)
	if yt.runs.Load() != before {
		t.Fatalf("looked up again")
	}
	trDo(t, "GET", srv.URL+"/api/v1/trailer/batch", "", 400)
	trDo(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=artist:1", "", 400)
	trDo(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:x", "", 400)
	trDo(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	trDo(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11", "", 404)
}

// trailerBatchView is the answer of GET /trailer/batch as the tests read it.
type trailerBatchView struct {
	Streams map[string]metadata.TrailerStream `json:"streams"`
	Videos  map[string]music.ArtistVideoView  `json:"videos"`
	Pending []string                          `json:"pending"`
}
