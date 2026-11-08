package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A cut list (the chosen edition of several) keeps its tracks through the
// page flight, and the next ask joins it instead of asking again.
func TestPageFlightKeepsCutList(t *testing.T) {
	t.Parallel()
	a := &API{}
	ctx := context.Background()
	asks := 0
	f := func(context.Context) ([]metadata.Pressing, error) {
		asks++
		return []metadata.Pressing{{MBID: "x", Tracks: []metadata.Track{{Name: "one"}}}}, metadata.ErrListCut
	}
	ps, err, answered := waitFlight[[]metadata.Pressing](ctx, pageFlight(a, ctx, "pressings:x", f), time.Now().Add(5*time.Second))
	if !answered || !errors.Is(err, metadata.ErrListCut) || len(ps) != 1 || len(ps[0].Tracks) != 1 {
		t.Fatalf("cut list lost: %v %v %v", ps, err, answered)
	}
	_, _, _ = waitFlight[[]metadata.Pressing](ctx, pageFlight(a, ctx, "pressings:x", f), time.Now().Add(5*time.Second))
	if asks != 1 {
		t.Fatalf("asked %d times, want 1 (joined)", asks)
	}
}
