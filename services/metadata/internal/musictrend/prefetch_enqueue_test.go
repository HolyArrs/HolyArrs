package musictrend

import (
	"context"
	"sort"
	"sync"
	"testing"
)

// A chart artist the pass could not settle (a busy source on every try)
// goes to the picture queue; one with a photo or a recorded miss does not.
// Between passes Sweep does the same for artists new on the charts.
func TestPrefetchQueuesUnsettledChartArtists(t *testing.T) {
	pics := &fakePictures{have: map[string]string{}, missed: map[string]bool{}, answer: map[string]string{}, busy: map[string]int{}}
	pics.answer["artist|"+id(1)] = "applemusic"
	pics.busy["artist|"+id(2)] = 99        // never settles this pass
	chart := []string{id(1), id(2), id(3)} // id(3): no source has it (miss)
	p := newPrefetch(pics, chart, nil, nil)
	var mu sync.Mutex
	var queued []string
	p.Enqueue = func(kind, mbid string) {
		mu.Lock()
		queued = append(queued, kind+"|"+mbid)
		mu.Unlock()
	}
	p.Pass(context.Background())
	if len(queued) != 1 || queued[0] != "artist|"+id(2) {
		t.Fatalf("queued %v, want only the unsettled artist", queued)
	}
	// A new chart artist between passes is queued by the sweep, without a
	// fetch of its own.
	queued = nil
	chart = append(chart, id(4))
	p.Artists = func(context.Context) []string { return chart }
	before := pics.fetches
	if n := p.Sweep(context.Background()); n != 2 {
		t.Fatalf("sweep queued %d, want 2", n)
	}
	sort.Strings(queued)
	if queued[0] != "artist|"+id(2) || queued[1] != "artist|"+id(4) || pics.fetches != before {
		t.Fatalf("sweep %v fetches %d", queued, pics.fetches-before)
	}
}
