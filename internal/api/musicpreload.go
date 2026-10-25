package api

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/music/rowtpl"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// Music cover preload (Luis, 2026-10-24: everything the UI will show is
// fetched before anyone opens the page). At start, whenever a new catalog
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
// musicPreloadGap between items (tests shorten it).
const musicPreloadWorkers = 4

var musicPreloadGap = 250 * time.Millisecond

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
	st      musicPreloadStatus
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
func (a *API) tplPreload(b *rowtpl.Bundle, region string) {
	if b == nil || !musicPreloadOn || !a.musicTplOn() || !a.partIsOn(parts.Music) {
		return
	}
	if a.musicPre.warm == nil && (a.s.Library == nil || a.s.Library.Artwork == nil) {
		return
	}
	p := &a.musicPre
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
		p.done, p.queue, p.seen = map[string]bool{}, nil, map[preloadKey]bool{}
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
		go supervisor.Guard("music-cover-preload", a.runPreload)
	}
}

// tplPreloadStatus answers the preload's progress.
func (a *API) tplPreloadStatus() musicPreloadStatus {
	p := &a.musicPre
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.st
	st.Regions = append([]string{}, p.st.Regions...)
	return st
}

// runPreload works through the queued regions, newest bundle first.
func (a *API) runPreload() {
	p := &a.musicPre
	for {
		p.mu.Lock()
		if len(p.queue) == 0 {
			p.running, p.st.Running = false, false
			p.st.Finished = time.Now().UTC()
			p.mu.Unlock()
			return
		}
		region, b, ver := p.queue[0], p.bundle, p.version
		p.queue = p.queue[1:]
		if p.st.Started.IsZero() {
			p.st.Started = time.Now().UTC()
		}
		p.mu.Unlock()
		keys := a.preloadItems(b, region, time.Now().UTC())
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
		a.preloadWarm(ver, todo)
	}
}

// preloadItems answers the albums and artists of a region's engine pages:
// the Browse tile covers and what each page shows now first, then every
// item their rows can draw on.
func (a *API) preloadItems(b *rowtpl.Bundle, region string, now time.Time) []preloadKey {
	ctx := metadata.Ahead(context.Background())
	r := strings.ToLower(region)
	if r == "" {
		r = "global"
	}
	sh := a.musicShelvesOf(ctx, strings.ToUpper(r), now)
	blocks := tplBlocks(sh)
	secs := a.tplVisible(ctx, b, r, nil, now)
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
func (a *API) preloadWarm(ver string, todo []preloadKey) {
	p := &a.musicPre
	warm := p.warm
	if warm == nil {
		warm = a.preloadOne
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
				ctx, cancel := context.WithTimeout(metadata.Ahead(context.Background()), artWarmItem)
				ok := warm(ctx, k)
				cancel()
				p.mu.Lock()
				if p.version == ver {
					if ok {
						p.st.Warmed++
					} else {
						p.st.Failed++
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

// preloadOne stores an album's covers at the card sizes (250 and 500) or an
// artist's photo, through the picture route's own chain.
func (a *API) preloadOne(ctx context.Context, k preloadKey) bool {
	if k.kind == "album" {
		_, _, small := a.musicTrendArtFile(ctx, "album", k.mbid, "250")
		_, _, big := a.musicTrendArtFile(ctx, "album", k.mbid, "500")
		return small && big
	}
	_, _, ok := a.musicTrendArtFile(ctx, "artist", k.mbid, "250")
	return ok
}
