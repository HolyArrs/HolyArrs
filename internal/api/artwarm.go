package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
)

// Artwork warm: every artist and album a row or page lists gets its
// pictures stored before anyone opens it. These are the same requests the
// page would make, only earlier: the artist's photo through the picture
// route, the artist's art list (wide photo, logo) and the album covers. A
// low-priority pool of artWarmWorkers does it; an item already warmed
// lately is skipped, and nothing here ever waits on a request.
const (
	artWarmWorkers = 4
	// artWarmFresh is how long a warmed item counts as fresh.
	artWarmFresh = 6 * time.Hour
	// artWarmMax caps the waiting list and the remembered items.
	artWarmMax = 5000
	// artWarmItem bounds one item's pictures.
	artWarmItem = 40 * time.Second
)

// artWarmGap is the wait between one worker's items (tests shorten it).
var artWarmGap = 150 * time.Millisecond

type artWarmKey struct{ kind, mbid string }

type artWarmer struct {
	mu      sync.Mutex
	pending []artWarmKey
	queued  map[artWarmKey]bool
	done    map[artWarmKey]time.Time
	workers int
	// do warms one item; nil means the real pictures (tests replace it).
	do func(ctx context.Context, k artWarmKey)
}

// warmArtItems queues albums and artists (kind, MusicBrainz id) for the
// background pool, after asking their picture records (a batch call per
// hundred) so the warm stores the versioned copies pages ask for.
func (a *API) warmArtItems(list []nameKey) {
	if len(list) == 0 || !a.partIsOn(parts.Music) {
		return
	}
	a.warmPictureRecords(list)
	a.wantPackages(list)
	w := &a.artWarm
	if w.do == nil && (a.s.Library == nil || a.s.Library.Artwork == nil) {
		return
	}
	now := time.Now()
	w.mu.Lock()
	if w.queued == nil || w.done == nil {
		w.queued, w.done = map[artWarmKey]bool{}, map[artWarmKey]time.Time{}
	}
	if len(w.done) > artWarmMax {
		w.done = map[artWarmKey]time.Time{}
	}
	for _, n := range list {
		k := artWarmKey{n.kind, strings.ToLower(n.mbid)}
		if (k.kind != "artist" && k.kind != "album") || !metadata.ValidMBID(k.mbid) || w.queued[k] || len(w.pending) >= artWarmMax {
			continue
		}
		if at, ok := w.done[k]; ok && now.Sub(at) < artWarmFresh {
			continue
		}
		w.queued[k] = true
		w.pending = append(w.pending, k)
	}
	for w.workers < artWarmWorkers && w.workers < len(w.pending) {
		w.workers++
		go a.artWarmWorker()
	}
	w.mu.Unlock()
}

// warmArtPaths queues the items behind picture routes (/api/v1/music-trending/art/{kind}/{mbid}).
func (a *API) warmArtPaths(imgs ...string) {
	list := make([]nameKey, 0, len(imgs))
	for _, img := range imgs {
		if rest, ok := strings.CutPrefix(img, musicArtPrefix); ok {
			kind, mbid, _ := strings.Cut(rest, "/")
			list = append(list, nameKey{kind: kind, mbid: mbid})
		}
	}
	a.warmArtItems(list)
}

func (a *API) artWarmWorker() {
	w := &a.artWarm
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
			do = a.warmArtItem
		}
		// While the metadata server refuses, background warms wait.
		metadata.WaitAhead(context.Background())
		ctx, cancel := context.WithTimeout(metadata.Ahead(context.Background()), artWarmItem)
		// The item's records may still be on their way (asked with the list).
		a.awaitPictureRecords(ctx, k.kind, k.mbid, 2*time.Second)
		do(ctx, k)
		cancel()
		w.mu.Lock()
		delete(w.queued, k)
		w.done[k] = time.Now()
		w.mu.Unlock()
		time.Sleep(artWarmGap)
	}
}

// warmArtItem stores one item's pictures: the photo or covers through the
// picture route (a stored file costs nothing), and an artist's art list.
func (a *API) warmArtItem(ctx context.Context, k artWarmKey) {
	switch k.kind {
	case "album":
		a.musicTrendArtFile(ctx, "album", k.mbid, "500")
	case "artist":
		a.musicTrendArtFile(ctx, "artist", k.mbid, "500")
		a.warmArtistExtras(ctx, k.mbid)
	}
}

// warmArtistExtras stores the artist's wide background and logo from the
// metadata server (the artist page asks for them first); one the server
// has not stored yet is asked again on a later warm.
func (a *API) warmArtistExtras(ctx context.Context, mbid string) {
	a.musicTrendArtFile(ctx, "wide", mbid, "1920")
	a.musicTrendArtFile(ctx, "logo", mbid, "1200")
}
