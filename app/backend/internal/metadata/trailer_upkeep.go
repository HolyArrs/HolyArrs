package metadata

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
)

const (
	trailerUpkeepKey   = "trailerhealth|reader" // the health check's state in lookup_cache
	trailerUpkeepKeep  = 90 * 24 * time.Hour    // ...kept this long after its last write
	trailerUpkeepVideo = 50                     // different failing titles and videos remembered
	trailerUpkeepStale = 7 * time.Hour          // no good lookup for this long: first lookups count too (an address lives about 6 hours)
)

// trailerUpkeep is the health check's state as kept in lookup_cache.
type trailerUpkeep struct {
	Videos   []string       `json:"videos,omitempty"`
	Reason   youtube.Reason `json:"reason,omitempty"`
	LastGood time.Time      `json:"last_good,omitzero"`
}

// TrailerReaderState is the YouTube reader's state for the health check.
type TrailerReaderState struct {
	Failed int // lookups that failed in a row at YouTube's door
	// Videos is how many different titles and music videos failed since the
	// last good lookup, for any reason but one about that video alone
	// (upkeepReason); a title counts once, however many trailers it tried.
	Videos     int
	LastGood   time.Time      // the last lookup that found addresses (zero: none yet)
	LastReason youtube.Reason // the reason of the last failure Videos counts ("" when none)
}

// upkeepReason says whether a failure for r counts toward the health
// check: every reason but one that is plainly about that video alone (age,
// sign-in, a content warning, a bad key). When YouTube changes, videos that
// played before come back without an address, refused, behind a bot check,
// unplayable or unavailable, across many videos at once.
func upkeepReason(r youtube.Reason) bool {
	switch r {
	case youtube.AgeRestricted, youtube.SignInNeeded, youtube.ContentCheck, youtube.BadID:
		return false
	}
	return true
}

// note counts a lookup's outcome under key (a title, or a music video): a
// success resets both counts, a door failure adds one to the failures in a
// row, and a failure for an upkeep reason adds key to the different ones
// that failed, so a title whose trailers all fail counts once. A video
// reason (removed, blocked) counts only when played says the title played
// here before (a renewal): when YouTube changes, trailers that played stop
// playing, while a title that never played (a known miss looked up again
// after a restart, say) says nothing about the reader. It says whether the
// health state should be written now: on a change, or at most once a
// minute for a success that changed nothing but the time.
func (t *Trailers) note(key string, err error, now time.Time, played bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		t.failed.Store(0)
		t.lastGood = now
		if len(t.upkeep) > 0 {
			t.upkeep = nil
			return true
		}
		return now.Sub(t.saved) >= time.Minute
	}
	r := TrailerReasonOf(err)
	if r == "" {
		r = youtube.Network
	}
	if !r.VideoReason() {
		t.failed.Add(1)
	}
	if !upkeepReason(r) || (r.VideoReason() && !t.countsVideoLocked(played, now)) {
		return false
	}
	changed := r != t.lastReason
	t.lastReason = r
	if !slices.Contains(t.upkeep, key) && len(t.upkeep) < trailerUpkeepVideo {
		t.upkeep = append(t.upkeep, key)
		changed = true
	}
	return changed
}

// loadUpkeep reads the health state kept before a restart, once, before
// anything counts.
func (t *Trailers) loadUpkeep(ctx context.Context) {
	t.loaded.Do(func() {
		if t.Store == nil || t.Store.Read() == nil {
			return
		}
		b, ok, err := LookupCache{Store: t.Store}.Get(context.WithoutCancel(ctx), trailerUpkeepKey, time.Now())
		var k trailerUpkeep
		if err != nil || !ok || json.Unmarshal(b, &k) != nil {
			return
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		t.upkeep = k.Videos[:min(len(k.Videos), trailerUpkeepVideo)]
		t.lastReason, t.lastGood = k.Reason, k.LastGood
	})
}

// saveUpkeep writes the health state as it is now (one write at a time, so
// an older state never lands after a newer one).
func (t *Trailers) saveUpkeep(ctx context.Context) {
	if t.Store == nil {
		return
	}
	t.saving.Lock()
	defer t.saving.Unlock()
	now := time.Now()
	t.mu.Lock()
	k := trailerUpkeep{Videos: slices.Clone(t.upkeep), Reason: t.lastReason, LastGood: t.lastGood}
	t.saved = now
	t.mu.Unlock()
	if b, err := json.Marshal(k); err == nil {
		_ = LookupCache{Store: t.Store}.Put(context.WithoutCancel(ctx), trailerUpkeepKey, b, now, now.Add(trailerUpkeepKeep))
	}
}

// countsVideo says whether a video reason counts toward the health check
// now: on a renewal (the title played here before), or when nothing played
// for longer than an address lives (the machine was off, trailers were
// turned off), since then nothing is a renewal any more.
func (t *Trailers) countsVideo(played bool, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.countsVideoLocked(played, now)
}

func (t *Trailers) countsVideoLocked(played bool, now time.Time) bool {
	return played || t.lastGood.IsZero() || now.Sub(t.lastGood) > trailerUpkeepStale
}

// On says whether trailers are looked up at all: every YouTube lookup (the
// trailer pass, music videos) waits on trailer autoplay, so the health
// check is off while it is off.
func (t *Trailers) On(ctx context.Context) bool {
	return t.Store == nil || t.Settings.AutoplayTrailers(ctx)
}

// ReaderState reports the reader's state (for the health check), with what
// was kept before a restart.
func (t *Trailers) ReaderState() TrailerReaderState {
	t.loadUpkeep(context.Background())
	t.mu.Lock()
	defer t.mu.Unlock()
	return TrailerReaderState{Failed: int(t.failed.Load()), Videos: len(t.upkeep), LastGood: t.lastGood, LastReason: t.lastReason}
}
