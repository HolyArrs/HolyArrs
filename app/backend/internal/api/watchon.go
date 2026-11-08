package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// watchOnTTL is how long a title's TMDB watch providers are kept per
// region; watchOnMiss how long a failed read is remembered.
const (
	watchOnTTL  = 24 * time.Hour
	watchOnMiss = 10 * time.Minute
	// watchOnBatch caps the reads one answer queues; watchOnMax the kept
	// titles.
	watchOnBatch = 60
	watchOnMax   = 20000
)

type watchOnEntry struct {
	at  time.Time
	ok  bool
	ids []int
	// names are the services' names, same order as ids.
	names []string
}

// watchOnCache keeps, per region|kind:tmdb, the subscription services TMDB
// lists for a title. Reads never wait: a miss is queued and filled in the
// background for the next answer.
type watchOnCache struct {
	mu      sync.Mutex
	titles  map[string]watchOnEntry
	pending map[string]bool
	// fetch is the TMDB read (tests replace it).
	fetch func(ctx context.Context, kind, tmdbID string) (map[string][]metadata.WatchService, error)
}

// watchOnKept returns the kept answer of kind:tmdb in region, and whether
// one is kept (a failure counts while it is remembered; see e.ok).
func (a *API) watchOnKept(region, key string, now time.Time) (watchOnEntry, bool) {
	c := &a.watchOn
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.titles[region+"|"+key]
	if !ok || (e.ok && now.Sub(e.at) >= watchOnTTL) || (!e.ok && now.Sub(e.at) >= watchOnMiss) {
		return watchOnEntry{}, false
	}
	return e, true
}

// queueWatchOn reads the missing titles' watch providers in the
// background (one read per title fills every region).
func (a *API) queueWatchOn(keys []string) {
	c := &a.watchOn
	fetch := c.fetch
	if fetch == nil {
		if a.s.Metadata == nil {
			return
		}
		fetch = func(ctx context.Context, kind, id string) (map[string][]metadata.WatchService, error) {
			l, ok := metadata.WatchListerFor(ctx, a.s.Metadata)
			if !ok {
				return nil, metadata.ErrNoAccess
			}
			return l.WatchProviders(ctx, kind, id)
		}
	}
	c.mu.Lock()
	if c.pending == nil {
		c.pending = map[string]bool{}
	}
	var todo []string
	for _, k := range keys {
		if len(todo) == watchOnBatch {
			break
		}
		if !c.pending[k] {
			c.pending[k] = true
			todo = append(todo, k)
		}
	}
	c.mu.Unlock()
	if len(todo) == 0 {
		return
	}
	go jobs.Guard("watch-on", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for _, k := range todo {
			kind, id, _ := strings.Cut(k, ":")
			got, err := fetch(ctx, kind, id)
			a.keepWatchOn(k, got, err, time.Now())
		}
	})
}

// keepWatchOn stores one title's answer for every region it lists; a
// failure is remembered under the "" region for watchOnMiss.
func (a *API) keepWatchOn(key string, got map[string][]metadata.WatchService, err error, now time.Time) {
	c := &a.watchOn
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, key)
	if c.titles == nil || len(c.titles) > watchOnMax {
		c.titles = map[string]watchOnEntry{}
	}
	if err != nil {
		c.titles["|"+key] = watchOnEntry{at: now}
		return
	}
	c.titles["|"+key] = watchOnEntry{at: now, ok: true}
	for region, list := range got {
		e := watchOnEntry{at: now, ok: true}
		for _, s := range list {
			e.ids = append(e.ids, int(s.ID))
			e.names = append(e.names, s.Name)
		}
		c.titles[region+"|"+key] = e
	}
}

// watchOnMarks maps each of keys (kind:tmdb) to the first of the member's
// services (in their order) TMDB lists the title on in region. Titles not
// kept yet are queued and left out.
func (a *API) watchOnMarks(region string, services []int, keys []string) map[string]streamingMark {
	if region == "" || len(services) == 0 || len(keys) == 0 {
		return nil
	}
	now := time.Now()
	out := map[string]streamingMark{}
	var miss []string
	for _, k := range keys {
		e, ok := a.watchOnKept(region, k, now)
		if !ok {
			// A title read (or failed) lately but absent from region
			// streams nowhere there; only an unread title is queued.
			if _, read := a.watchOnKept("", k, now); !read {
				miss = append(miss, k)
			}
			continue
		}
	pick:
		for _, want := range services {
			for i, id := range e.ids {
				if id == want {
					out[k] = streamingMark{Service: id, Name: e.names[i]}
					break pick
				}
			}
		}
	}
	if len(miss) > 0 {
		a.queueWatchOn(miss)
	}
	return out
}
