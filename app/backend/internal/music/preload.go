package music

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music/rowtpl"
)

// Music cover preload: everything the UI will show is fetched before
// anyone opens the page. At start, whenever a new catalog
// bundle arrives and when a region is first asked for, every album and
// artist an engine page can show has its pictures stored: every shown
// Browse page, the Browse tile covers, Explore's engine rows and New &
// Popular, as a member with an empty library sees them. What the pages show
// now goes first, then everything their rows can draw on. A few paced
// workers do it in the background; no page waits for it. Progress is in
// /api/v1/health under music_covers.

// musicPreloadOn turns the preload on (tests turn it off, see TestMain).
var musicPreloadOn = true

// musicPreloadWorkers items are warmed at once; each worker waits
// musicPreloadGap between items (tests shorten it): about 4 items (8
// pictures) a second, under the server's per-install limit. A 429 holds
// every worker back for musicPreloadPause.
const musicPreloadWorkers = 2

var (
	musicPreloadGap   = 500 * time.Millisecond
	musicPreloadPause = 30 * time.Second
)

// slowDown holds the preload back after a 429 from the server.
func (p *musicPreload) slowDown(now time.Time) {
	p.mu.Lock()
	if until := now.Add(musicPreloadPause); until.After(p.pauseUntil) {
		p.pauseUntil = until
	}
	p.mu.Unlock()
}

// waitPause sleeps while the preload is held back.
func (p *musicPreload) waitPause() {
	for {
		p.mu.Lock()
		d := time.Until(p.pauseUntil)
		p.mu.Unlock()
		if d <= 0 {
			return
		}
		time.Sleep(d)
	}
}

type preloadKey struct{ kind, mbid string }

type musicPreload struct {
	// catalog is the last catalog bundle read: the picture route answers
	// any album or artist in it.
	catalog atomic.Pointer[rowtpl.Bundle]
	mu      sync.Mutex
	bundle  *rowtpl.Bundle
	version string
	// asked holds every region pages were asked for; done the regions
	// preloaded (or queued) for this bundle version.
	asked   map[string]bool
	done    map[string]bool
	queue   []string
	running bool
	seen    map[preloadKey]bool
	// missed holds the items the server had no picture for yet; they are
	// asked again musicPreloadRetry later (the server stores them meanwhile).
	missed []preloadKey
	// have holds the items whose pictures are stored (kept across bundle
	// versions); engine rows show only those once a preload has run.
	have map[preloadKey]bool
	// haveReady is set when a preload pass first finished: before that
	// (start, the first pass running) rows are not checked against have.
	haveReady bool
	// pauseUntil holds the preload back after the server said 429.
	pauseUntil time.Time
	st         musicPreloadStatus
	// warm stores one item's pictures; nil means the real ones (tests
	// replace it).
	warm func(ctx context.Context, k preloadKey) bool
}

// musicPreloadStatus is the preload's progress for the health page.
type musicPreloadStatus struct {
	Version  string    `json:"version"`
	Regions  []string  `json:"regions"`
	Running  bool      `json:"running"`
	Total    int       `json:"total"`
	Warmed   int       `json:"warmed"`
	Failed   int       `json:"failed"`
	Started  time.Time `json:"started,omitzero"`
	Finished time.Time `json:"finished,omitzero"`
}

// tplPreload asks for the covers of a region's pages for bundle b ("" asks
// for none, only the bundle check): a new bundle version preloads every
// region asked so far again; a region already done for b is skipped.
func (h *Handlers) tplPreload(b *rowtpl.Bundle, region string) {
	if b == nil || !musicPreloadOn || !h.musicTplOn() || !h.PartIsOn(parts.Music) {
		return
	}
	if h.musicPre.warm == nil && (h.lib() == nil || h.lib().Artwork == nil) {
		return
	}
	p := &h.musicPre
	r := strings.ToLower(region)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.asked == nil {
		p.asked = map[string]bool{}
	}
	if r != "" {
		p.asked[r] = true
	}
	if p.version != b.Version {
		p.bundle, p.version = b, b.Version
		p.done, p.queue, p.seen, p.missed = map[string]bool{}, nil, map[preloadKey]bool{}, nil
		p.st = musicPreloadStatus{Version: b.Version}
	}
	for x := range p.asked {
		if !p.done[x] {
			p.done[x] = true
			p.queue = append(p.queue, x)
			p.st.Regions = append(p.st.Regions, x)
		}
	}
	if len(p.queue) > 0 && !p.running {
		p.running, p.st.Running = true, true
		go jobs.Guard("music-cover-preload", h.runPreload)
	}
}

func (p *musicPreload) markLocked(k preloadKey) {
	if p.have == nil {
		p.have = map[preloadKey]bool{}
	}
	p.have[k] = true
}

// tplMarkArt records a stored picture (the picture route stored it).
func (h *Handlers) tplMarkArt(kind, mbid string) {
	if kind != "album" && kind != "artist" {
		return
	}
	p := &h.musicPre
	p.mu.Lock()
	p.markLocked(preloadKey{kind, mbid})
	p.mu.Unlock()
}

// artKnown reports a picture that is stored: the preload or the picture
// route stored it, or its file is in the artwork store (then it is noted).
func (h *Handlers) artKnown(k preloadKey) bool {
	p := &h.musicPre
	p.mu.Lock()
	v := p.have[k]
	p.mu.Unlock()
	if !v && h.localArt(k) {
		v = true
		p.mu.Lock()
		p.markLocked(k)
		p.mu.Unlock()
	}
	return v
}

// artReady reports that a preload has stored a picture, so a missing one means
// "not stored" and no longer "not looked at yet".
func (h *Handlers) artReady() bool {
	p := &h.musicPre
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.haveReady || len(p.have) > 0
}

// tplHasArt answers the engine's picture check for bundle b: an item whose
// picture is stored (the preload or the picture route stored it, or its
// file is in the artwork store). nil (no check) until a preload has stored
// any picture, so a page never empties before the first one runs.
func (h *Handlers) tplHasArt(b *rowtpl.Bundle) func(int) bool {
	if !h.artReady() {
		return nil
	}
	memo := map[int]bool{}
	return func(i int) bool {
		if v, ok := memo[i]; ok {
			return v
		}
		it := b.Items[i]
		k := preloadKey{"album", it.ID}
		if it.Kind == "r" {
			k.kind = "artist"
		}
		v := h.artKnown(k)
		memo[i] = v
		return v
	}
}

// albumHasArt answers the same check for an album id (a chart's, not in the
// bundle); nil until a preload has stored any picture.
func (h *Handlers) albumHasArt() func(mbid string) bool {
	if !h.artReady() {
		return nil
	}
	return func(mbid string) bool { return h.artKnown(preloadKey{"album", strings.ToLower(mbid)}) }
}

// localArt reports a picture already in the artwork store (the server's
// copy, at a card size).
func (h *Handlers) localArt(k preloadKey) bool {
	lib := h.lib()
	if lib == nil || lib.Artwork == nil {
		return false
	}
	kind := "album"
	if k.kind == "artist" {
		kind = "artist"
	}
	// The card copy: at the known version, or the unversioned one.
	for _, v := range []string{h.PictureVersion(kind, k.mbid), ""} {
		if src := h.musicArtSrc(kind, k.mbid, "500", v); src != "" {
			if _, ok := lib.Artwork.Path(library.ArtworkName(src)); ok {
				return true
			}
		}
	}
	return false
}

// tplPreloadStatus answers the preload's progress.
func (h *Handlers) tplPreloadStatus() musicPreloadStatus {
	p := &h.musicPre
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.st
	st.Regions = append([]string{}, p.st.Regions...)
	return st
}

// runPreload works through the queued regions, newest bundle first.
func (h *Handlers) runPreload() {
	p := &h.musicPre
	for {
		p.mu.Lock()
		if len(p.queue) == 0 {
			p.running, p.st.Running = false, false
			p.st.Finished = time.Now().UTC()
			p.haveReady = true
			if len(p.missed) > 0 {
				ver := p.version
				time.AfterFunc(musicPreloadRetry, func() { h.retryPreload(ver) })
			}
			p.mu.Unlock()
			return
		}
		region, b, ver := p.queue[0], p.bundle, p.version
		p.queue = p.queue[1:]
		if p.st.Started.IsZero() {
			p.st.Started = time.Now().UTC()
		}
		p.mu.Unlock()
		keys := h.preloadItems(b, region, time.Now().UTC())
		p.mu.Lock()
		if p.version != ver {
			p.mu.Unlock()
			continue
		}
		var todo []preloadKey
		for _, k := range keys {
			if !p.seen[k] {
				p.seen[k] = true
				todo = append(todo, k)
			}
		}
		p.st.Total += len(todo)
		p.mu.Unlock()
		h.preloadWarm(ver, todo)
	}
}

// preloadItems answers the albums and artists of a region's engine pages:
// the Browse tile covers and what each page shows now first, then every
// item their rows can draw on.
func (h *Handlers) preloadItems(b *rowtpl.Bundle, region string, now time.Time) []preloadKey {
	ctx := metadata.Ahead(context.Background())
	r := strings.ToLower(region)
	if r == "" {
		r = "global"
	}
	sh := h.musicShelvesOf(ctx, strings.ToUpper(r), now)
	blocks := tplBlocks(sh)
	secs := h.tplVisible(ctx, b, r, nil, now)
	var out []preloadKey
	addID := func(kind, id string) {
		if id != "" {
			out = append(out, preloadKey{kind, id})
		}
	}
	add := func(i int) {
		it := b.Items[i]
		if it.Kind == "r" {
			addID("artist", it.ID)
			return
		}
		addID("album", it.ID)
	}
	for _, s := range secs {
		for _, t := range s.Tiles {
			for _, c := range t.Covers {
				addID("album", strings.TrimPrefix(c, musicArtPrefix+"album/"))
			}
		}
	}
	type page struct{ kind, key string }
	pages := []page{{"explore", ""}, {"new", ""}}
	for _, k := range rowtpl.ShownKeys(secs) {
		pages = append(pages, page{kindOfKey(k), k})
	}
	ctxOf := func(pg page) *rowtpl.Ctx {
		c := &rowtpl.Ctx{B: b, Kind: pg.kind, Key: pg.key, Region: r, Now: now, Blocks: blocks, CountryName: countryName}
		if sh != nil {
			c.Neighbor = sh.Neighbor
		}
		return c
	}
	for _, pg := range pages {
		c := ctxOf(pg)
		for _, i := range rowtpl.Spotlight(c) {
			add(i)
			// The Spotlight shows the artist's photo over the cover.
			addID("artist", b.Items[i].ArtistID)
		}
		for _, row := range rowtpl.Build(c) {
			for _, i := range row.Items {
				add(i)
			}
		}
	}
	for _, pg := range pages {
		for _, i := range rowtpl.PageItems(ctxOf(pg)) {
			add(i)
		}
	}
	return out
}

// preloadWarm stores the items' pictures with a few paced workers; a newer
// bundle stops it.
func (h *Handlers) preloadWarm(ver string, todo []preloadKey) {
	p := &h.musicPre
	warm := p.warm
	if warm == nil {
		warm = h.preloadOne
	}
	stale := func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.version != ver
	}
	next := make(chan preloadKey)
	var wg sync.WaitGroup
	for range musicPreloadWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range next {
				p.waitPause()
				ctx, cancel := context.WithTimeout(metadata.Ahead(context.Background()), ArtWarmItem)
				ok := warm(ctx, k)
				cancel()
				p.mu.Lock()
				if p.version == ver {
					if ok {
						p.st.Warmed++
						p.markLocked(k)
					} else {
						p.st.Failed++
						p.missed = append(p.missed, k)
					}
				}
				p.mu.Unlock()
				if musicPreloadGap > 0 {
					time.Sleep(musicPreloadGap)
				}
			}
		}()
	}
	for _, k := range todo {
		if stale() {
			break
		}
		next <- k
	}
	close(next)
	wg.Wait()
}

// musicPreloadRetry is when the pictures the server had not stored yet are
// asked again (past the picture route's miss time).
var musicPreloadRetry = musicArtMissTTL + time.Minute

// retryPreload asks again for the items the last pass missed, while the
// bundle is the same; misses again are retried again later.
func (h *Handlers) retryPreload(ver string) {
	p := &h.musicPre
	p.mu.Lock()
	if p.version != ver || p.running || len(p.missed) == 0 {
		p.mu.Unlock()
		return
	}
	todo := p.missed
	p.missed = nil
	p.st.Failed -= len(todo)
	p.running, p.st.Running = true, true
	p.mu.Unlock()
	go jobs.Guard("music-cover-preload-retry", func() {
		h.preloadWarm(ver, todo)
		h.runPreload() // closes the run (and schedules the next retry)
	})
}

// preloadOne stores an album's cover or an artist's photo at the card size
// (500; a 250 thumbnail is made here from it), through the picture route's
// own chain.
func (h *Handlers) preloadOne(ctx context.Context, k preloadKey) bool {
	kind := "album"
	if k.kind != "album" {
		kind = "artist"
	}
	_, _, ok := h.musicTrendArtFile(ctx, kind, k.mbid, "500")
	return ok
}
