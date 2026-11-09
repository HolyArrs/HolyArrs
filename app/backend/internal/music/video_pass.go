package music

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Artists' music videos on the trailer pass's schedule: the artist
// page and the in-view batch only note an artist; the pass makes one of
// its videos ready (the stream address only) and keeps it ready, renewing
// it before it expires, for as long as the artist is asked for. A page
// never starts a lookup and never waits on one. A click on one video in
// the Music videos row is the exception: a person pressed play on it.

const (
	artistAskedMax  = 300                // artists kept ready at most (the one asked for least lately goes)
	artistAskedKeep = 7 * 24 * time.Hour // an artist nobody asked for in this long is let go
	artistAskedDisk = "artistvideos|asked"
	// artistEmptyRecheck is how often the pass asks the metadata server
	// again about a noted artist that has no videos (a page open asks
	// sooner, see artistVideosEmpty).
	artistEmptyRecheck = 24 * time.Hour
	// artistFailWait is how long the pass leaves an artist alone after
	// asking the metadata server about it failed (the server down).
	artistFailWait = time.Hour
)

// artistAsked are the artists whose music videos the pass keeps ready, by
// MusicBrainz id: when each was last asked for, and in which order.
type artistAsked struct {
	mu     sync.Mutex
	m      map[string]artistAsk
	n      uint64
	loaded bool
	dirty  bool
}

// artistAsk is when an artist was last asked for, and in which order.
type artistAsk struct {
	at time.Time
	n  uint64
}

type storedArtistAsk struct {
	MBID string    `json:"mbid"`
	At   time.Time `json:"at"`
	N    uint64    `json:"n"`
}

// loadArtistAsks reads the kept list once, so a restart keeps the artists
// ready (their streams are kept in SQLite too).
func (h *Handlers) loadArtistAsks(ctx context.Context) {
	t := &h.artistAsked
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.loaded {
		return
	}
	t.loaded = true
	if t.m == nil {
		t.m = map[string]artistAsk{}
	}
	if h.Store == nil {
		return
	}
	b, found, _ := metadata.LookupCache{Store: h.Store}.Get(ctx, artistAskedDisk, time.Now())
	var kept []storedArtistAsk
	if !found || json.Unmarshal(b, &kept) != nil {
		return
	}
	for _, k := range kept {
		if metadata.ValidMBID(k.MBID) && len(t.m) < artistAskedMax {
			t.m[k.MBID] = artistAsk{at: k.At, n: k.N}
			t.n = max(t.n, k.N)
		}
	}
}

// saveArtistAsks writes the list when it changed.
func (h *Handlers) saveArtistAsks(ctx context.Context) {
	t := &h.artistAsked
	t.mu.Lock()
	if !t.dirty || h.Store == nil {
		t.mu.Unlock()
		return
	}
	t.dirty = false
	kept := make([]storedArtistAsk, 0, len(t.m))
	for k, e := range t.m {
		kept = append(kept, storedArtistAsk{MBID: k, At: e.at, N: e.n})
	}
	t.mu.Unlock()
	if b, err := json.Marshal(kept); err == nil {
		now := time.Now()
		_ = metadata.LookupCache{Store: h.Store}.Put(ctx, artistAskedDisk, b, now, now.Add(artistAskedKeep))
	}
}

// noteArtistVideos notes artists for the pass; no lookup here. One noted
// again keeps its place; when full, the one asked for least lately goes.
func (h *Handlers) noteArtistVideos(ctx context.Context, mbids []string) {
	if len(mbids) == 0 || !h.ArtistVideosOn() {
		return
	}
	h.loadArtistAsks(ctx)
	t := &h.artistAsked
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	var fresh []string
	for _, id := range mbids {
		if !metadata.ValidMBID(id) {
			continue
		}
		if e, ok := t.m[id]; ok {
			e.at = now
			t.m[id] = e
			continue
		}
		if !slices.Contains(fresh, id) {
			fresh = append(fresh, id)
		}
	}
	for _, id := range fresh {
		if len(t.m) >= artistAskedMax {
			oldest := ""
			for k, e := range t.m {
				if o, ok := t.m[oldest]; !ok || e.at.Before(o.at) || (e.at.Equal(o.at) && e.n < o.n) {
					oldest = k
				}
			}
			delete(t.m, oldest)
		}
		t.n++
		t.m[id] = artistAsk{at: now, n: t.n}
	}
	t.dirty = true
}

// artistAsks are the noted artists, first asked first; ones nobody asked
// for in artistAskedKeep are let go.
func (h *Handlers) artistAsks(ctx context.Context, now time.Time) []string {
	h.loadArtistAsks(ctx)
	t := &h.artistAsked
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []string
	for k, e := range t.m {
		if now.Sub(e.at) > artistAskedKeep {
			delete(t.m, k)
			t.dirty = true
			continue
		}
		out = append(out, k)
	}
	slices.SortFunc(out, func(x, y string) int { return cmp.Compare(t.m[x].n, t.m[y].n) })
	return out
}

// artistVideoDue says whether the pass looks one of the artist's videos up:
// none ready yet (and one not tried), or the ready one's address expires
// within trailerRenewAhead (a renewal; urgent when it would lapse before
// the next pass).
func (h *Handlers) artistVideoDue(ctx context.Context, mbid string, now time.Time) (due, renew, urgent bool) {
	// No videos when last asked: not asked again before artistEmptyRecheck,
	// so an install's noted artists cost the metadata server about one call
	// a day each, not one every pass.
	if e, kept := h.keptVideos(ctx, mbid); kept && len(e.list) == 0 && h.artistVideos.clock().Sub(e.at) < artistEmptyRecheck {
		return false, false, false
	}
	c := &h.artistVideos
	c.mu.Lock()
	failedAt, failed := c.failed[mbid]
	c.mu.Unlock()
	if failed && c.clock().Sub(failedAt) < artistFailWait {
		return false, false, false
	}
	vids := h.videosOf(ctx, mbid)
	if len(vids) == 0 {
		return false, false, false
	}
	if v, s, ok := h.readyVideo(ctx, vids); ok {
		if h.Trailers.InFlight("musicvideo|" + v.ID) {
			return false, false, false
		}
		left := s.ExpiresAt.Add(-metadata.TrailerEarly).Sub(now)
		return left < h.Trailers.RenewAhead(), true, left < h.Trailers.RenewAhead()-metadata.TrailerPassEvery()
	}
	for _, v := range vids {
		if _, known := h.videoKnown(ctx, v.ID); !known && !h.Trailers.InFlight("musicvideo|"+v.ID) {
			return true, false, false
		}
	}
	return false, false, false
}

// passArtistVideo makes the artist's video ready (the first one not tried)
// or renews the ready one. A video YouTube will not play here is dropped
// from the artist's list; a renewal that fails keeps the address it has
// until it expires.
func (h *Handlers) passArtistVideo(ctx context.Context, mbid string, renew bool) error {
	vids := h.videosOf(ctx, mbid)
	if renew {
		v, _, ok := h.readyVideo(ctx, vids)
		if !ok {
			return nil
		}
		ckey := "musicvideo|" + v.ID
		now := time.Now()
		s, err := h.Trailers.Read(metadata.Renewing(ctx), v.ID, v.ID, now, true)
		if err != nil {
			if videoWontPlay(metadata.MissedFor(err)) {
				h.dropVideo(ctx, mbid, v.ID)
			}
			return metadata.MissedFor(err)
		}
		h.Trailers.KeepStream(ctx, ckey, s, now)
		return nil
	}
	for _, v := range vids {
		if _, known := h.videoKnown(ctx, v.ID); known {
			continue
		}
		_, err := h.resolveMusicVideo(ctx, v.ID)
		if videoWontPlay(err) && ctx.Err() == nil {
			h.dropVideo(ctx, mbid, v.ID)
		}
		return err
	}
	return nil
}
