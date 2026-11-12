package discover

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TitleWarmFresh is how long a warmed title is left alone; kept extras are
// refreshed on the same clock.
const TitleWarmFresh = 6 * time.Hour

// TitleWarmer fills title details, extras and trailer links ahead, on the
// server's own schedule, for every title Explore and New & Popular list,
// so no visitor action makes the first outside call. Two workers at most.
// Trailer addresses are not looked up here (the trailer pass does that).
// Its state is zero-value ready; set the funcs before the first Warm.
type TitleWarmer struct {
	Store    func() *database.Store
	Metadata func() metadata.Provider
	Cache    func() *Cache
	// WarmExtras stores a title's extras; TrailerKey learns its trailer key.
	WarmExtras func(ctx context.Context, kind, id string)
	TrailerKey func(ctx context.Context, kind, id, lang string)

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

// Warm queues titles (kind movie or show, TMDB id) to be warmed; a title
// warmed within TitleWarmFresh is skipped. It never blocks.
func (w *TitleWarmer) Warm(lang string, titles [][2]string) {
	w.once.Do(func() {
		w.queue = make(chan warmJob, 1000)
		w.listed = map[string]warmMark{}
		for range warmWorkers {
			go w.worker()
		}
		go w.schedule()
	})
	now := time.Now()
	for _, t := range titles {
		if (t[0] != "movie" && t[0] != "show") || !metadata.DigitsOnly(t[1]) {
			continue
		}
		k := t[0] + "|" + t[1] + "|" + lang
		w.mu.Lock()
		m := w.listed[k]
		fresh := now.Sub(m.warmed) < TitleWarmFresh
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

func (w *TitleWarmer) schedule() {
	tick := time.NewTicker(warmEvery)
	defer tick.Stop()
	for range tick.C {
		now := time.Now()
		var due []warmJob
		w.mu.Lock()
		for k, m := range w.listed {
			if now.Sub(m.listed) > warmListed {
				delete(w.listed, k)
				continue
			}
			if now.Sub(m.warmed) >= TitleWarmFresh {
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

func (w *TitleWarmer) worker() {
	for j := range w.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		w.warmOne(ctx, j)
		cancel()
	}
}

// warmOne stores one title's details and extras (when missing or old) and
// learns its trailer's key.
func (w *TitleWarmer) warmOne(ctx context.Context, j warmJob) {
	store, c := w.Store(), w.Cache()
	if d, ok := metadata.DiscovererFor(ctx, w.Metadata()); ok {
		key := "details|" + j.kind + "|" + j.id
		e, cached := c.Get(key)
		if !cached || e.Details == nil {
			if k, found := metadata.LoadKept[metadata.Details](ctx, store, key); found {
				e, cached = Entry{At: k.At, Details: &k.V}, true
				c.Put(key, e)
			}
		}
		// Details kept before titles carried their texts are read again.
		if !cached || e.Details == nil || time.Since(e.At) >= DetailsTTL(e) || !textKept(ctx, w.Metadata(), j.kind, j.id) {
			if det, err := d.Details(ctx, j.kind, j.id); err == nil {
				c.Put(key, Entry{At: time.Now(), Details: &det})
				metadata.KeepOnDisk(ctx, store, key, det)
			}
		}
	}
	w.WarmExtras(ctx, j.kind, j.id)
	if !health.AutoplayOn(ctx, store) {
		return
	}
	// The key only (the metadata server's pick, else TMDB's videos; no
	// YouTube call), for the cards and the hero's gate.
	w.TrailerKey(metadata.Ahead(ctx), j.kind, j.id, j.lang)
}
