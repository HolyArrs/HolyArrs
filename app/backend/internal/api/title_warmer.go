package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// titleWarmFresh is how long a warmed title is left alone; kept extras are
// refreshed on the same clock.
const titleWarmFresh = 6 * time.Hour

// keepOnDisk stores v under key in lookup_cache with when it was fetched.
func keepOnDisk[T any](ctx context.Context, a *API, key string, v T) {
	metadata.KeepOnDisk(ctx, a.s.Store, key, v)
}

// loadKept reads an answer keepOnDisk stored, as a memory entry.
func loadKept[T any](ctx context.Context, a *API, key string) (struct {
	v  T
	at time.Time
}, bool) {
	var out struct {
		v  T
		at time.Time
	}
	k, ok := metadata.LoadKept[T](ctx, a.s.Store, key)
	out.v, out.at = k.V, k.At
	return out, ok
}

// titleWarmer fills title details, extras and trailer links ahead, on the
// server's own schedule, for every title Explore and New & Popular list,
// so no visitor action makes the first outside call. Two workers at most.
// Trailer addresses are not looked up here (the trailer pass does that).
type titleWarmer struct {
	once  sync.Once
	mu    sync.Mutex
	queue chan warmJob
	// listed is when each title was last listed and warmed.
	listed map[string]warmMark
}

type warmJob struct{ kind, id, lang string }

type warmMark struct{ listed, warmed time.Time }

const (
	warmWorkers = 2
	warmEvery   = time.Hour          // the schedule re-warms what went stale
	warmListed  = 7 * 24 * time.Hour // titles listed this recently stay warm
	warmMax     = 5000
)

// warmTitles queues titles (kind movie or show, TMDB id) to be warmed; a
// title warmed within titleWarmFresh is skipped. It never blocks.
func (a *API) warmTitles(lang string, titles [][2]string) {
	w := &a.warm
	w.once.Do(func() {
		w.queue = make(chan warmJob, 1000)
		w.listed = map[string]warmMark{}
		for range warmWorkers {
			go a.warmWorker()
		}
		go a.warmSchedule()
	})
	now := time.Now()
	for _, t := range titles {
		if (t[0] != "movie" && t[0] != "show") || !metadata.DigitsOnly(t[1]) {
			continue
		}
		k := t[0] + "|" + t[1] + "|" + lang
		w.mu.Lock()
		m := w.listed[k]
		fresh := now.Sub(m.warmed) < titleWarmFresh
		m.listed = now
		if !fresh {
			m.warmed = now // queued; a failure is tried again on the schedule
		}
		if len(w.listed) < warmMax || w.listed[k] != (warmMark{}) {
			w.listed[k] = m
		}
		w.mu.Unlock()
		if fresh {
			continue
		}
		select {
		case w.queue <- warmJob{t[0], t[1], lang}:
		default: // full: the schedule picks it up later
		}
	}
}

func (a *API) warmSchedule() {
	tick := time.NewTicker(warmEvery)
	defer tick.Stop()
	for range tick.C {
		now := time.Now()
		var due []warmJob
		w := &a.warm
		w.mu.Lock()
		for k, m := range w.listed {
			if now.Sub(m.listed) > warmListed {
				delete(w.listed, k)
				continue
			}
			if now.Sub(m.warmed) >= titleWarmFresh {
				parts := strings.SplitN(k, "|", 3)
				due = append(due, warmJob{parts[0], parts[1], parts[2]})
				m.warmed = now
				w.listed[k] = m
			}
		}
		w.mu.Unlock()
		for _, j := range due {
			w.queue <- j
		}
	}
}

func (a *API) warmWorker() {
	for j := range a.warm.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		a.warmOne(ctx, j)
		cancel()
	}
}

// warmOne stores one title's details and extras (when missing or old) and
// learns its trailer's key.
func (a *API) warmOne(ctx context.Context, j warmJob) {
	if d, ok := metadata.DiscovererFor(ctx, a.s.Metadata); ok {
		key := "details|" + j.kind + "|" + j.id
		e, cached := a.discover.get(key)
		if !cached || e.details == nil {
			if k, found := loadKept[metadata.Details](ctx, a, key); found {
				e, cached = discoverEntry{at: k.at, details: &k.v}, true
				a.discover.put(key, e)
			}
		}
		if !cached || e.details == nil || time.Since(e.at) >= detailsTTL(e) {
			if det, err := d.Details(ctx, j.kind, j.id); err == nil {
				a.discover.put(key, discoverEntry{at: time.Now(), details: &det})
				keepOnDisk(ctx, a, key, det)
			}
		}
	}
	a.titles.WarmExtras(ctx, j.kind, j.id)
	if !a.uiSettings(ctx).AutoplayTrailers {
		return
	}
	// The key only (the metadata server's pick, else TMDB's videos; no
	// YouTube call), for the cards and the hero's gate.
	_, _ = a.trailers.Key(metadata.Ahead(ctx), j.kind, j.id, j.lang)
}
