package refresh

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// Defaults for the runner.
const (
	DefaultEvery       = time.Hour
	DefaultWikiEvery   = 6 * time.Hour
	DefaultBackstop    = 14 * 24 * time.Hour
	DefaultEvictAfter  = 180 * 24 * time.Hour
	DefaultHotWindow   = 30 * 24 * time.Hour
	DefaultBudget      = 300
	DefaultDiskWarning = 0.75
)

// RevisionChecker is the Wikidata lastrevid check.
type RevisionChecker interface {
	CheckRevisions(context.Context) (int, error)
}

// Runner drives the feeds, background refetches, the backstop, eviction
// of long-unrequested rows and the disk alarm.
type Runner struct {
	Tracker  *Tracker
	TMDB     *TMDBPoller
	MB       *MBFeed
	Wikidata RevisionChecker
	// Refetch refreshes one row (through the server's budgeted fetch).
	Refetch func(context.Context, Item) error
	// Evict deletes rows from the answer store.
	Evict func(context.Context, []string) error
	// Budget is the most refetches per run (default 300).
	Budget int
	// Every is the feed cadence (default 1 h); WikiEvery the Wikidata one (6 h).
	Every, WikiEvery time.Duration
	// Backstop reverifies rows not verified for this long (default 14 d).
	Backstop time.Duration
	// EvictAfter deletes rows not requested for this long (default 180 d;
	// negative turns eviction off).
	EvictAfter time.Duration
	// MediaRecheck, when set, marks popular TMDB titles stale on that
	// cadence (images and videos the change feed does not report).
	MediaRecheck time.Duration
	// DiskPath is the store volume watched by the disk alarm.
	DiskPath    string
	DiskWarning float64
	// StoreBytes reports the store size on disk.
	StoreBytes func() int64
	Now        func() time.Time
	Logf       func(string, ...any)
}

func (r *Runner) defaults() {
	if r.Budget <= 0 {
		r.Budget = DefaultBudget
	}
	if r.Every <= 0 {
		r.Every = DefaultEvery
	}
	if r.WikiEvery <= 0 {
		r.WikiEvery = DefaultWikiEvery
	}
	if r.Backstop <= 0 {
		r.Backstop = DefaultBackstop
	}
	if r.EvictAfter == 0 {
		r.EvictAfter = DefaultEvictAfter
	}
	if r.DiskWarning <= 0 {
		r.DiskWarning = DefaultDiskWarning
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Logf == nil {
		r.Logf = log.Printf
	}
}

// Once runs one feed cycle; it returns how many rows were refetched.
func (r *Runner) Once(ctx context.Context) int {
	r.defaults()
	now := r.Now()
	if r.TMDB != nil {
		if n, err := r.TMDB.Poll(ctx); err != nil {
			r.Logf("refresh: tmdb changes failed: %v", err)
		} else if n > 0 {
			r.Logf("refresh: tmdb changes marked %d rows stale", n)
		}
	}
	if r.MB.Enabled() {
		if n, err := r.MB.Poll(ctx); err != nil {
			r.Logf("refresh: musicbrainz replication failed: %v", err)
		} else if n > 0 {
			r.Logf("refresh: musicbrainz replication marked %d rows stale", n)
		}
	}
	if r.MediaRecheck > 0 {
		r.mediaRecheck(ctx, now)
	}
	done := 0
	if r.Refetch != nil {
		// Changed rows asked for recently first, then the backstop. Rows
		// not asked for recently stay marked and refresh on request.
		pending, _ := r.Tracker.Pending(ctx, now.Add(-DefaultHotWindow), r.Budget)
		for _, it := range pending {
			if ctx.Err() != nil || r.Refetch(ctx, it) != nil {
				break
			}
			done++
		}
		if done < r.Budget {
			due, _ := r.Tracker.Backstop(ctx, now.Add(-r.Backstop), r.Budget-done)
			for _, it := range due {
				if ctx.Err() != nil || r.Refetch(ctx, it) != nil {
					break
				}
				done++
			}
		}
	}
	if r.EvictAfter > 0 && r.Evict != nil {
		old, _ := r.Tracker.Unrequested(ctx, now.Add(-r.EvictAfter), 1000)
		if len(old) > 0 {
			keys := make([]string, len(old))
			for i, it := range old {
				keys[i] = it.Key
			}
			if r.Evict(ctx, keys) == nil {
				r.Tracker.Forget(ctx, keys)
			}
		}
	}
	for _, w := range r.Snapshot(ctx).Warnings {
		r.Logf("refresh: warning: %s", w)
	}
	return done
}

func (r *Runner) mediaRecheck(ctx context.Context, now time.Time) {
	last, _ := time.Parse(time.RFC3339, r.Tracker.Get(ctx, "media_recheck"))
	if now.Sub(last) < r.MediaRecheck {
		return
	}
	hot, err := r.Tracker.Popular(ctx, "tmdb:", now.Add(-7*24*time.Hour), 500)
	if err != nil {
		return
	}
	keys := make([]string, 0, len(hot))
	for _, it := range hot {
		if !strings.HasPrefix(it.Ref, "tmdb:person:") {
			keys = append(keys, it.Key)
		}
	}
	r.Tracker.MarkKeys(ctx, keys)
	r.Tracker.Set(ctx, "media_recheck", now.UTC().Format(time.RFC3339))
}

// Snapshot is the refresh stats with the disk alarm.
func (r *Runner) Snapshot(ctx context.Context) Stats {
	r.defaults()
	st := r.Tracker.Snapshot(ctx)
	if r.StoreBytes != nil {
		st.StoreBytes = r.StoreBytes()
	}
	if !r.MB.Enabled() {
		st.Feeds["musicbrainz"] = "disabled_no_token"
	}
	if r.DiskPath != "" {
		if used, total, err := DiskUsage(r.DiskPath); err == nil && total > 0 {
			d := &Disk{UsedBytes: used, TotalBytes: total, UsedRatio: float64(used) / float64(total)}
			st.Disk = d
			if d.UsedRatio >= r.DiskWarning {
				st.Warnings = append(st.Warnings, fmt.Sprintf("disk_usage_high: store volume %.0f%% used", d.UsedRatio*100))
			}
		}
	}
	return st
}

// Run loops: feeds every Every, Wikidata every WikiEvery.
func (r *Runner) Run(ctx context.Context) {
	r.defaults()
	var lastWiki time.Time
	for {
		r.Once(ctx)
		if r.Wikidata != nil && r.Now().Sub(lastWiki) >= r.WikiEvery {
			if n, err := r.Wikidata.CheckRevisions(ctx); err != nil {
				r.Logf("refresh: wikidata revisions failed: %v", err)
			} else {
				lastWiki = r.Now()
				if n > 0 {
					r.Logf("refresh: wikidata revisions marked %d answers stale", n)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.Every):
		}
	}
}
