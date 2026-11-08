package music

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Artwork warm queue: every artist and album a row or page lists gets its
// pictures stored before anyone opens it. A low-priority pool of
// ArtWarmWorkers runs the caller's warm; an item warmed lately is skipped,
// and nothing here ever waits on a request.
const (
	ArtWarmWorkers = 4
	// artWarmFresh is how long a warmed item counts as fresh.
	artWarmFresh = 6 * time.Hour
	// artWarmMax caps the waiting list and the remembered items.
	artWarmMax = 5000
	// ArtWarmItem bounds one item's pictures.
	ArtWarmItem = 40 * time.Second
)

// ArtWarmGap is the wait between one worker's items (tests shorten it).
var ArtWarmGap = 150 * time.Millisecond

// ArtWarmKey names one warm item: kind "artist" or "album" and its MBID.
type ArtWarmKey struct{ Kind, MBID string }

type artWarmer struct {
	mu      sync.Mutex
	pending []ArtWarmKey
	queued  map[ArtWarmKey]bool
	done    map[ArtWarmKey]time.Time
	workers int
	// do warms one item; nil means the caller's warm (tests replace it).
	do func(ctx context.Context, k ArtWarmKey)
}

// SetArtWarmDo replaces the warm of one item (tests).
func (h *Handlers) SetArtWarmDo(do func(ctx context.Context, k ArtWarmKey)) {
	h.artWarm.mu.Lock()
	h.artWarm.do = do
	h.artWarm.mu.Unlock()
}

// HasArtWarmDo says a test warm is set.
func (h *Handlers) HasArtWarmDo() bool {
	h.artWarm.mu.Lock()
	defer h.artWarm.mu.Unlock()
	return h.artWarm.do != nil
}

// ArtWarmIdle says no worker runs and nothing waits.
func (h *Handlers) ArtWarmIdle() bool {
	h.artWarm.mu.Lock()
	defer h.artWarm.mu.Unlock()
	return h.artWarm.workers == 0 && len(h.artWarm.pending) == 0
}

// QueueArtWarm queues artist and album keys and starts workers that run
// warm (or the test warm) for each, after await gives the item's records
// a moment to arrive.
func (h *Handlers) QueueArtWarm(keys []ArtWarmKey, await, warm func(ctx context.Context, k ArtWarmKey)) {
	w := &h.artWarm
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.queued == nil || w.done == nil {
		w.queued, w.done = map[ArtWarmKey]bool{}, map[ArtWarmKey]time.Time{}
	}
	if len(w.done) > artWarmMax {
		w.done = map[ArtWarmKey]time.Time{}
	}
	for _, k := range keys {
		k.MBID = strings.ToLower(k.MBID)
		if (k.Kind != "artist" && k.Kind != "album") || !metadata.ValidMBID(k.MBID) || w.queued[k] || len(w.pending) >= artWarmMax {
			continue
		}
		if at, ok := w.done[k]; ok && now.Sub(at) < artWarmFresh {
			continue
		}
		w.queued[k] = true
		w.pending = append(w.pending, k)
	}
	for w.workers < ArtWarmWorkers && w.workers < len(w.pending) {
		w.workers++
		go h.artWarmWorker(await, warm)
	}
}

func (h *Handlers) artWarmWorker(await, warm func(ctx context.Context, k ArtWarmKey)) {
	w := &h.artWarm
	for {
		w.mu.Lock()
		if len(w.pending) == 0 {
			w.workers--
			w.mu.Unlock()
			return
		}
		k := w.pending[0]
		w.pending = w.pending[1:]
		do := w.do
		w.mu.Unlock()
		if do == nil {
			do = warm
		}
		// While the metadata server refuses, background warms wait.
		metadata.WaitAhead(context.Background())
		ctx, cancel := context.WithTimeout(metadata.Ahead(context.Background()), ArtWarmItem)
		await(ctx, k)
		do(ctx, k)
		cancel()
		w.mu.Lock()
		delete(w.queued, k)
		w.done[k] = time.Now()
		w.mu.Unlock()
		time.Sleep(ArtWarmGap)
	}
}
