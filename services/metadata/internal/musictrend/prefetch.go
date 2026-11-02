package musictrend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// Chart picture prefetch and the publish gate (music data plan S5, AC 10
// and 24). Every chart artist (new releases count as a chart) plus the
// top similar artists of each gets its photo and every album cover stored
// through the live picture chain (Apple Music first, overflow per the
// picture decision). Charts stay hidden ("Building") until the gate
// passes: every artist has a photo, and every album has a cover or a
// recorded miss.

// SimilarPerArtist is how many similar artists of each chart artist are
// prefetched.
const SimilarPerArtist = 5

// Pictures is the picture store the prefetch fills (imgstore.Store).
type Pictures interface {
	// Provider answers the stored picture's source; false when none.
	Provider(ctx context.Context, kind, id string) (string, bool)
	// Missed reports a recorded "no source has it".
	Missed(ctx context.Context, kind, id string) bool
	// Fetch walks the picture chain and stores the picture.
	Fetch(ctx context.Context, kind, id string) error
}

// Prefetch fills the chart pictures and keeps the gate.
type Prefetch struct {
	Pictures Pictures
	// Artists answers every chart artist now (charts, trending lists, genre
	// pages and new releases); no call.
	Artists func(ctx context.Context) []string
	// Similar answers an artist's similar artists, best first, from the
	// stored sources (Apple Music similar, else ListenBrainz).
	Similar func(ctx context.Context, mbid string) []string
	// Albums answers an artist's albums (release group MBIDs) from the
	// slim MusicBrainz copy; no call.
	Albums func(ctx context.Context, mbid string) []string
	// Workers is how many pictures are fetched at once (default 8); the
	// picture chain paces each host itself.
	Workers int
	// Retries is how many passes a picture that could not be fetched now
	// (a busy or failing source) gets (default 3).
	Retries int
	// Every is how often the prefetch runs (default 6 h, after the chart
	// refresh).
	Every time.Duration
	// ProgressEvery is the progress log interval (default 30 s).
	ProgressEvery time.Duration
	// Start is the wait before the first pass.
	Start time.Duration
	// Log reports progress (nil: none).
	Log func(format string, args ...any)
	// LiveFile marks that the charts went live once (written when the
	// gate first opens, read at start). Once live, a later pass never
	// closes the gate again: the last good charts keep being served.
	// "": kept in memory only.
	LiveFile string
	// Paused reports a fetch error that only means a source is paused or
	// rate-limited (nil: none counts). An artist blocked only by that for
	// PendingAfter counts as a temporary miss and is retried next pass.
	Paused func(err error) bool
	// Enqueue puts one picture in the picture queue (kind "artist"): every
	// chart artist a pass leaves with neither a photo nor a recorded miss,
	// and (Sweep) artists new on the charts between passes. nil: none.
	Enqueue func(kind, mbid string)
	// SweepEvery is how often Run sweeps between passes (default 15 min).
	SweepEvery   time.Duration
	PendingAfter time.Duration // default 30 min
	Now          func() time.Time

	ready     atomic.Bool
	live      atomic.Bool
	pendMu    sync.Mutex
	pendSince map[string]time.Time // picture ("kind:id") blocked by a paused source since
	last      atomic.Pointer[Coverage]
	runMu     sync.Mutex
}

// Coverage is one prefetch pass's result.
type Coverage struct {
	At            time.Time      `json:"at"`
	Took          string         `json:"took"`
	ChartArtists  int            `json:"chart_artists"`
	Artists       int            `json:"artists"` // chart plus similar
	ArtistPhotos  int            `json:"artist_photos"`
	ArtistMisses  int            `json:"artist_misses"`  // every source tried, miss recorded
	ArtistPending int            `json:"artist_pending"` // blocked by a paused source past PendingAfter (temporary miss)
	Albums        int            `json:"albums"`
	AlbumCovers   int            `json:"album_covers"`
	AlbumMisses   int            `json:"album_misses"`
	AlbumPending  int            `json:"album_pending"` // blocked by a paused source past PendingAfter (temporary miss)
	Fetched       int            `json:"fetched"`
	Failed        int            `json:"failed"`
	PhotoSources  map[string]int `json:"photo_sources"`
	CoverSources  map[string]int `json:"cover_sources"`
	NoPhoto       []string       `json:"no_photo,omitempty"` // every artist with a recorded miss
	Open          []string       `json:"open,omitempty"`     // first few albums with neither
	Ready         bool           `json:"ready"`
	ChartArtistID []string       `json:"-"`
}

// Ready reports the gate: the charts went live once, or the last pass
// covered every picture.
func (p *Prefetch) Ready() bool {
	if p.ready.Load() || p.live.Load() {
		return true
	}
	if p.LiveFile != "" {
		if _, err := os.Stat(p.LiveFile); err == nil {
			p.live.Store(true)
			return true
		}
	}
	return false
}

func (p *Prefetch) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// pendKey is a picture's key in pendSince.
func pendKey(kind, id string) string { return kind + ":" + id }

// notePaused records a picture ("kind:id") blocked by a paused source (or
// clears it).
func (p *Prefetch) notePaused(id string, paused bool) {
	p.pendMu.Lock()
	defer p.pendMu.Unlock()
	if p.pendSince == nil {
		p.pendSince = map[string]time.Time{}
	}
	if !paused {
		delete(p.pendSince, id)
	} else if _, ok := p.pendSince[id]; !ok {
		p.pendSince[id] = p.now()
	}
}

// pending reports a picture ("kind:id") blocked by a paused source for
// PendingAfter.
func (p *Prefetch) pending(id string) bool {
	after := p.PendingAfter
	if after <= 0 {
		after = 30 * time.Minute
	}
	p.pendMu.Lock()
	defer p.pendMu.Unlock()
	at, ok := p.pendSince[id]
	return ok && p.now().Sub(at) >= after
}

// Last answers the last pass (nil before the first).
func (p *Prefetch) Last() *Coverage { return p.last.Load() }

// Members answers the artists a pass covers: the chart artists, then the
// top SimilarPerArtist similar artists of each, without repeats.
func (p *Prefetch) Members(ctx context.Context) (chart, all []string) {
	seen := map[string]bool{}
	for _, a := range p.Artists(ctx) {
		if validMBID(a) && !seen[a] {
			seen[a] = true
			chart = append(chart, a)
		}
	}
	all = append(all, chart...)
	if p.Similar == nil {
		return chart, all
	}
	// Similar lookups run side by side; each source's client keeps its
	// own pace (Apple on one address at 10/s, ListenBrainz its limit).
	sims := make([][]string, len(chart))
	var next, done atomic.Int64
	var wg sync.WaitGroup
	stop := p.every30s(func() { p.logf("chart prefetch: similar artists %d/%d", done.Load(), len(chart)) })
	for range p.workers() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(chart) || ctx.Err() != nil {
					return
				}
				sims[i] = p.Similar(ctx, chart[i])
				done.Add(1)
			}
		}()
	}
	wg.Wait()
	stop()
	for i := range chart {
		n := 0
		for _, s := range sims[i] {
			if n >= SimilarPerArtist {
				break
			}
			if !validMBID(s) {
				continue
			}
			n++
			if !seen[s] {
				seen[s] = true
				all = append(all, s)
			}
		}
	}
	return chart, all
}

func (p *Prefetch) workers() int {
	if p.Workers <= 0 {
		return 8
	}
	return p.Workers
}

func (p *Prefetch) logf(format string, args ...any) {
	if p.Log != nil {
		p.Log(format, args...)
	}
}

// every30s calls f every ProgressEvery (30 s) until the returned stop.
func (p *Prefetch) every30s(f func()) (stop func()) {
	every := p.ProgressEvery
	if every <= 0 {
		every = 30 * time.Second
	}
	ch := make(chan struct{})
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ch:
				return
			case <-t.C:
				f()
			}
		}
	}()
	return func() { close(ch) }
}

type picJob struct{ kind, id string }

// Pass fetches every missing picture and checks the gate. The gate holds
// (Ready) only when every artist has a photo and every album has a cover
// or a recorded miss; until then the charts stay hidden.
func (p *Prefetch) Pass(ctx context.Context) Coverage {
	p.runMu.Lock()
	defer p.runMu.Unlock()
	start := time.Now()
	// The gate needs the 500 card only. The big size (a second download,
	// decode and 1200 WebP encode) goes to the picture queue instead of
	// holding one of the two CPU slots inside every fetch.
	ctx = imgstore.BigLater(ctx)
	workers, retries := p.workers(), p.Retries
	if retries <= 0 {
		retries = 3
	}
	chart, all := p.Members(ctx)
	var jobs []picJob
	seenAlbum := map[string]bool{}
	for _, a := range all {
		jobs = append(jobs, picJob{"artist", a})
		if p.Albums == nil {
			continue
		}
		for _, g := range p.Albums(ctx, a) {
			if validMBID(g) && !seenAlbum[g] {
				seenAlbum[g] = true
				jobs = append(jobs, picJob{"album", g})
			}
		}
	}
	done := func(j picJob) bool {
		if _, ok := p.Pictures.Provider(ctx, j.kind, j.id); ok {
			return true
		}
		return p.Pictures.Missed(ctx, j.kind, j.id)
	}
	var fetched, failed, misses atomic.Int64
	var srcMu sync.Mutex
	bySource := map[string]int{}
	var handled atomic.Int64
	stopLog := p.every30s(func() {
		srcMu.Lock()
		src := fmt.Sprint(bySource)
		srcMu.Unlock()
		p.logf("chart prefetch: pictures %d/%d fetched, by source %s, misses %d, retry %d", handled.Load(), len(jobs), src, misses.Load(), failed.Load())
	})
	defer stopLog()
	todo := jobs
	for try := 0; try < retries && len(todo) > 0 && ctx.Err() == nil; try++ {
		var open []picJob
		for _, j := range todo {
			if !done(j) {
				open = append(open, j)
			}
		}
		if len(open) == 0 {
			break
		}
		if p.Log != nil {
			p.Log("chart prefetch: pass %d, %d of %d pictures to fetch", try+1, len(open), len(jobs))
		}
		ch := make(chan picJob)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var again []picJob
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range ch {
					err := p.Pictures.Fetch(ctx, j.kind, j.id)
					handled.Add(1)
					if err == nil {
						fetched.Add(1)
						if src, ok := p.Pictures.Provider(ctx, j.kind, j.id); ok {
							srcMu.Lock()
							bySource[j.kind+"/"+src]++
							srcMu.Unlock()
						}
						continue
					}
					if p.Pictures.Missed(ctx, j.kind, j.id) {
						misses.Add(1)
						continue
					}
					p.notePaused(pendKey(j.kind, j.id), p.Paused != nil && p.Paused(err))
					failed.Add(1)
					mu.Lock()
					again = append(again, j)
					mu.Unlock()
				}
			}()
		}
		for _, j := range open {
			if ctx.Err() != nil {
				break
			}
			ch <- j
		}
		close(ch)
		wg.Wait()
		todo = again
	}
	c := Coverage{At: time.Now().UTC(), ChartArtists: len(chart), Artists: len(all), Albums: len(seenAlbum),
		PhotoSources: map[string]int{}, CoverSources: map[string]int{}, Fetched: int(fetched.Load()), Failed: int(failed.Load()), ChartArtistID: chart}
	for _, j := range jobs {
		src, ok := p.Pictures.Provider(ctx, j.kind, j.id)
		switch {
		case j.kind == "artist" && ok:
			c.ArtistPhotos++
			c.PhotoSources[src]++
		case j.kind == "artist" && p.Pictures.Missed(ctx, j.kind, j.id):
			c.ArtistMisses++
			c.NoPhoto = append(c.NoPhoto, j.id)
		case j.kind == "artist" && p.pending(pendKey(j.kind, j.id)):
			c.ArtistPending++
		case j.kind == "artist":
			if len(c.Open) < 20 {
				c.Open = append(c.Open, j.id)
			}
		case ok:
			c.AlbumCovers++
			c.CoverSources[src]++
		case p.Pictures.Missed(ctx, j.kind, j.id):
			c.AlbumMisses++
		case p.pending(pendKey(j.kind, j.id)):
			c.AlbumPending++
		default:
			if len(c.Open) < 20 {
				c.Open = append(c.Open, j.id)
			}
		}
	}
	if p.Enqueue != nil {
		// Left open (a busy or paused source on every try): the queue
		// keeps asking, so the artist never waits for the next pass.
		for _, a := range all {
			if !done(picJob{"artist", a}) {
				p.Enqueue("artist", a)
			}
		}
	}
	sort.Strings(c.NoPhoto)
	c.Ready = Gate(c)
	c.Took = time.Since(start).Round(time.Second).String()
	p.last.Store(&c)
	p.ready.Store(c.Ready)
	if c.Ready && !p.live.Load() {
		p.live.Store(true)
		if p.LiveFile != "" {
			_ = os.WriteFile(p.LiveFile, []byte(c.At.Format(time.RFC3339)), 0o644)
		}
	}
	if p.Log != nil {
		p.Log("chart prefetch: %d chart artists, %d with similar; photos %d/%d %v, photo misses %d, pending %d; albums %d (covers %d %v, misses %d, pending %d); ready %v in %s",
			c.ChartArtists, c.Artists, c.ArtistPhotos, c.Artists, c.PhotoSources, c.ArtistMisses, c.ArtistPending, c.Albums, c.AlbumCovers, c.CoverSources, c.AlbumMisses, c.AlbumPending, c.Ready, c.Took)
	}
	return c
}

// Gate is the publish rule: at least one chart artist, and every artist
// and album with a picture or a recorded miss (every source in the chain
// tried; the miss keeps its source tags).
func Gate(c Coverage) bool {
	return c.ChartArtists > 0 && c.ArtistPhotos+c.ArtistMisses+c.ArtistPending == c.Artists && c.AlbumCovers+c.AlbumMisses+c.AlbumPending == c.Albums
}

// ErrBuilding is answered for a chart while the gate is closed.
var ErrBuilding = errors.New("musictrend: charts are building")

// Run passes after Start (the charts load and refresh first) and then
// every Every until ctx ends; a pass that does not open the gate runs
// again after a short wait.
func (p *Prefetch) Run(ctx context.Context) {
	every := p.Every
	if every <= 0 {
		every = 6 * time.Hour
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(p.Start):
	}
	sweep := p.SweepEvery
	if sweep <= 0 {
		sweep = 15 * time.Minute
	}
	for {
		c := p.Pass(ctx)
		wait := passWait(c.Ready, p.Ready(), every)
		if !c.Ready && p.Log != nil && len(c.Open) > 0 {
			p.Log("chart prefetch: still open %v", c.Open)
		}
		next := time.Now().Add(wait)
		for time.Until(next) > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(min(sweep, time.Until(next))):
			}
			if time.Until(next) > 0 {
				p.Sweep(ctx)
			}
		}
	}
}

// passWait is how long Run waits after a pass: the short retry only until
// the charts first go live (after that the Sweep queues the stragglers).
func passWait(passReady, live bool, every time.Duration) time.Duration {
	if passReady {
		return every
	}
	if live {
		return min(every, time.Hour)
	}
	return 10 * time.Minute
}

// Sweep queues every chart artist with neither a photo nor a recorded
// miss (no fetch here: the picture queue does it) and answers how many.
func (p *Prefetch) Sweep(ctx context.Context) int {
	if p.Enqueue == nil || p.Artists == nil {
		return 0
	}
	n := 0
	seen := map[string]bool{}
	for _, a := range p.Artists(ctx) {
		if !validMBID(a) || seen[a] {
			continue
		}
		seen[a] = true
		if _, ok := p.Pictures.Provider(ctx, "artist", a); ok || p.Pictures.Missed(ctx, "artist", a) {
			continue
		}
		p.Enqueue("artist", a)
		n++
	}
	return n
}
