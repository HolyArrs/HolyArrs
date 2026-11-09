package music

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// Music fetch-ahead: whenever the prebuild or a rebuild picks items for a
// music row or page (Explore Music, New & Popular Music, genre pages, the
// next pages of the plan), their details and pictures are fetched in
// the background before anyone opens them. An item fetched once is marked
// (memory and lookup_cache), so a rebuild that re-ranks the same items
// makes no fetch for them. Every request carries X-Fetch: ahead, follows
// the server's switch (on, slow, off) and backs off when the server asks.

// aheadGap spaces detail fetches (at most one a second per install) and
// aheadArtGap pictures; tests shorten both.
var (
	aheadGap    = time.Second
	aheadArtGap = musicArtWarmGap
)

const (
	// aheadDetailsMax caps detail fetches in one run (slow: half).
	aheadDetailsMax = 200
	// aheadKeep is how long an item fetched ahead counts as kept.
	aheadKeep = 7 * 24 * time.Hour
	// aheadArtistWait lets the full artist lookup (paged at MusicBrainz's
	// pace) finish, so the artist page reads it from the lookup cache.
	aheadArtistWait = 45 * time.Second
	// aheadFails stops a run after this many busy answers in a row.
	aheadFails = 3
	// musicControlEvery is how often the server's switch is read again.
	musicControlEvery = 15 * time.Minute
	// musicControlStale: with no good read for this long, act as slow.
	musicControlStale = 24 * time.Hour
)

// aheadBackoff is the first wait after a busy answer (doubles each time);
// tests shorten it.
var aheadBackoff = 30 * time.Second

type aheadItem struct{ kind, mbid string }

type musicAhead struct {
	mu      sync.Mutex
	ctx     context.Context // the Music loop's; nil while Music is off
	pending []aheadItem
	queued  map[aheadItem]bool
	running bool
	kept    map[aheadItem]time.Time
	// run lets one fetch-ahead pass go at a time.
	run sync.Mutex
}

type musicControl struct {
	mu    sync.Mutex
	c     metadata.Control
	good  time.Time // last good read
	tried time.Time
	since time.Time // first try
}

var aheadOn = metadata.Control{Prefetch: "on"}

// musicSwitch answers the server's switch, read at most every 15 minutes.
// An older server without one is on. A failed read keeps the last value;
// after a day without a good read the install acts as slow.
func (h *Handlers) musicSwitch(ctx context.Context, now time.Time) metadata.Control {
	if h.MusicControl == nil {
		return aheadOn
	}
	c := &h.ctl
	c.mu.Lock()
	if c.since.IsZero() {
		c.since = now
	}
	read := c.tried.IsZero() || now.Sub(c.tried) >= musicControlEvery
	if read {
		c.tried = now
	}
	c.mu.Unlock()
	if read {
		v, err := h.MusicControl(ctx)
		c.mu.Lock()
		switch {
		case err == nil:
			c.c, c.good = v.Normal(), now
		case errors.Is(err, metadata.ErrNotFound):
			c.c, c.good = aheadOn, now
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.good.IsZero() {
		if now.Sub(c.since) >= musicControlStale {
			return metadata.Control{Prefetch: "slow"}
		}
		return aheadOn
	}
	v := c.c
	if now.Sub(c.good) >= musicControlStale && v.Prefetch == "on" {
		v.Prefetch = "slow"
	}
	return v
}

// musicPrebuildGap is the wait before the next music prebuild: the 3 h
// (or the server's minimum, doubled when slow) give or take half of it at
// random (±90 min on 3 h), so installs do not all ask at once.
func musicPrebuildGap(c metadata.Control) time.Duration {
	base := musicPrebuildEvery
	if hv := time.Duration(c.PrebuildMinHours * float64(time.Hour)); hv > base {
		base = hv
	}
	if c.Prefetch == "slow" {
		base *= 2
	}
	return spread(base, 0.5)
}

// spread is d give or take frac of it, at random.
func spread(d time.Duration, frac float64) time.Duration {
	j := time.Duration(float64(d) * frac)
	if j <= 0 {
		return d
	}
	return d - j + time.Duration(rand.Int64N(int64(2*j)+1))
}

// spreadAt is a fixed spread for one kept entry (seeded by when it was
// kept), so the charts of different installs expire at different times.
func spreadAt(d time.Duration, at time.Time, frac float64) time.Duration {
	j := int64(float64(d) * frac)
	if j <= 0 {
		return d
	}
	r := rand.New(rand.NewPCG(uint64(at.UnixNano()), 0x6d75736963))
	return d - time.Duration(j) + time.Duration(r.Int64N(2*j+1))
}

// beginAhead lets fetch-ahead run under the Music loop's ctx; endAhead
// stops it when Music is turned off.
func (h *Handlers) beginAhead(ctx context.Context) {
	h.ahead.mu.Lock()
	h.ahead.ctx = ctx
	h.ahead.mu.Unlock()
}

func (h *Handlers) endAhead() {
	h.ahead.mu.Lock()
	h.ahead.ctx, h.ahead.pending, h.ahead.queued = nil, nil, nil
	h.ahead.mu.Unlock()
}

// queueAhead adds items for the background pass (a page served with more
// to come); nothing while Music's loop is not running.
func (h *Handlers) queueAhead(items []aheadItem) {
	hv := &h.ahead
	hv.mu.Lock()
	defer hv.mu.Unlock()
	if hv.ctx == nil || hv.ctx.Err() != nil || len(items) == 0 {
		return
	}
	if hv.queued == nil {
		hv.queued = map[aheadItem]bool{}
	}
	for _, it := range items {
		if !hv.queued[it] {
			hv.queued[it] = true
			hv.pending = append(hv.pending, it)
		}
	}
	if hv.running {
		return
	}
	hv.running = true
	ctx := hv.ctx
	go func() {
		for {
			hv.mu.Lock()
			l := hv.pending
			hv.pending, hv.queued = nil, nil
			if len(l) == 0 || ctx.Err() != nil {
				hv.running = false
				hv.mu.Unlock()
				return
			}
			hv.mu.Unlock()
			h.fetchAhead(ctx, l)
		}
	}()
}

// aheadKept says whether the item was fetched ahead lately.
func (h *Handlers) aheadKept(ctx context.Context, it aheadItem, now time.Time) bool {
	hv := &h.ahead
	hv.mu.Lock()
	at, ok := hv.kept[it]
	hv.mu.Unlock()
	if ok && now.Sub(at) < aheadKeep {
		return true
	}
	if h.Store == nil {
		return false
	}
	if _, ok, err := (metadata.LookupCache{Store: h.Store}).Get(ctx, "musicahead|"+it.kind+"|"+it.mbid, now); err == nil && ok {
		hv.mu.Lock()
		if hv.kept == nil {
			hv.kept = map[aheadItem]time.Time{}
		}
		hv.kept[it] = now
		hv.mu.Unlock()
		return true
	}
	return false
}

func (h *Handlers) keepAhead(ctx context.Context, it aheadItem, now time.Time) {
	hv := &h.ahead
	hv.mu.Lock()
	if hv.kept == nil || len(hv.kept) > 50000 {
		hv.kept = map[aheadItem]time.Time{}
	}
	hv.kept[it] = now
	hv.mu.Unlock()
	if h.Store != nil {
		_ = metadata.LookupCache{Store: h.Store}.Put(context.WithoutCancel(ctx), "musicahead|"+it.kind+"|"+it.mbid, []byte("1"), now, now.Add(aheadKeep))
	}
}

// aheadDetails fetches what the item's page reads first: the artist (its
// full album list lands in the lookup cache) or the album's pressings.
func (h *Handlers) aheadDetails(ctx context.Context, it aheadItem) error {
	ar := h.lib().Artists
	switch it.kind {
	case "artist":
		_, err := ar.Artist(metadata.QuickArtist(ctx, aheadArtistWait), it.mbid)
		return err
	case "album":
		// The page's own one-call album answer, never a fan-out over editions.
		_, err := ar.Pressings(metadata.MemberLookup(ctx), it.mbid)
		if errors.Is(err, metadata.ErrListCut) {
			return nil
		}
		return err
	}
	return nil
}

func aheadBusy(err error) bool {
	return errors.Is(err, metadata.ErrServiceBusy) || errors.Is(err, metadata.ErrServiceUnavailable) || errors.Is(err, metadata.ErrRateLimited)
}

func aheadGone(err error) bool {
	return errors.Is(err, metadata.ErrNotFound) || errors.Is(err, metadata.ErrArtistNotFound)
}

func aheadSleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// fetchAhead fetches the items' details (when not kept) and pictures (the
// picture route keeps files, so a kept one costs nothing), one at a time.
// The server's switch is read as it goes: off stops, slow doubles the gaps
// and halves the detail count. Busy answers back off (30 s, 1 min, 2 min,
// with jitter) and the run stops after three in a row.
func (h *Handlers) fetchAhead(ctx context.Context, items []aheadItem) {
	if len(items) == 0 {
		return
	}
	hv := &h.ahead
	hv.run.Lock()
	defer hv.run.Unlock()
	ctx = metadata.Ahead(ctx)
	details, fails := 0, 0
	canDetail := h.lib() != nil && h.lib().Artists != nil
	for _, it := range items {
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		c := h.musicSwitch(ctx, now)
		if c.Prefetch == "off" {
			return
		}
		gap, artGap, limit := aheadGap, aheadArtGap, aheadDetailsMax
		if c.Prefetch == "slow" {
			gap, artGap, limit = 2*gap, 2*artGap, limit/2
		}
		if c.MaxRPS > 0 {
			if g := time.Duration(float64(time.Second) / c.MaxRPS); g > gap {
				gap = g
			}
		}
		if canDetail && details < limit && !h.aheadKept(ctx, it, now) {
			details++
			err := h.aheadDetails(ctx, it)
			switch {
			case err == nil || aheadGone(err):
				fails = 0
				h.keepAhead(ctx, it, time.Now())
			case aheadBusy(err):
				fails++
				if fails >= aheadFails {
					return
				}
				if !aheadSleep(ctx, spread(aheadBackoff<<(fails-1), 0.25)) {
					return
				}
			}
			if !aheadSleep(ctx, gap) {
				return
			}
		}
		_, _, _ = h.musicTrendArtFile(ctx, it.kind, it.mbid, "500")
		if !aheadSleep(ctx, artGap) {
			return
		}
	}
}

// aheadOf turns (kind, mbid) pairs into fetch-ahead items, valid and once
// each, in order.
func aheadOf(pairs [][2]string) []aheadItem {
	seen := map[aheadItem]bool{}
	var out []aheadItem
	for _, p := range pairs {
		it := aheadItem{kind: p[0], mbid: strings.ToLower(p[1])}
		if (it.kind != "artist" && it.kind != "album") || !metadata.ValidMBID(it.mbid) || seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
	}
	return out
}

// cardsAhead lists the albums and artists music cards show.
func cardsAhead(cards []MusicCard) []aheadItem {
	var pairs [][2]string
	for _, c := range cards {
		switch c.Kind {
		case "album":
			pairs = append(pairs, [2]string{"album", c.MBID})
		case "artist":
			pairs = append(pairs, [2]string{"artist", c.MBID})
		}
		if c.ArtistMBID != "" {
			pairs = append(pairs, [2]string{"artist", c.ArtistMBID})
		}
	}
	return aheadOf(pairs)
}

// musicPagesAhead lists the items on n pages of a v7 plan from page
// from: the same rows the next page loads will serve.
func musicPagesAhead(p *musicscore.Plan, from, n int) []aheadItem {
	ord, ok := p.Meta.(*musicOrder)
	if !ok || ord == nil || ord.v7 == nil {
		return nil
	}
	var cards []MusicCard
	for k := from; k < from+n; k++ {
		rows, more := p.Page(k, nil)
		for _, row := range rows {
			for _, it := range row.Items {
				cards = append(cards, ord.v7.cards[row.ID][it.Key])
			}
		}
		if !more {
			break
		}
	}
	return cardsAhead(cards)
}
