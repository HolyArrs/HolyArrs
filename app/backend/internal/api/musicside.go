package api

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Music page side data (genres and links, bios, Deezer profiles, outside
// ids, top songs) is kept in memory and in lookup_cache, so a restart reads
// it back instead of asking again. An answer past its fresh time is served
// at once and asked again in the background; a miss (nothing known) is
// remembered for an hour. A cold lookup goes on after the page stopped
// waiting for it, so the next view finds it kept.

const (
	musicSidePrefix = "music_side:"
	// musicSideKeep is how long an answer may be served stale.
	musicSideKeep = 30 * 24 * time.Hour
	// musicSideMiss is how long a miss is remembered.
	musicSideMiss = time.Hour
	// musicSideFetch bounds one lookup (cold or in the background).
	musicSideFetch = 30 * time.Second
	musicSideMax   = 5000
)

// sideRow is one kept answer, in memory and on disk.
type sideRow struct {
	At   time.Time       `json:"at"`
	Miss bool            `json:"miss,omitempty"`
	V    json.RawMessage `json:"v,omitempty"`
}

// sideFlight is one lookup in progress; done closes when it ends.
type sideFlight struct {
	done  chan struct{}
	v     any
	found bool
	err   error
}

type sideStore struct {
	mu      sync.Mutex
	rows    map[string]sideRow
	flights map[string]*sideFlight
	// pages are the artist and album pages' own lookups in progress (and
	// for musicPageJoin after), so a page asked again joins them.
	pages map[string]*sideFlight
}

// musicPageJoin is how long a finished page lookup answers asks again
// (the page's quiet re-asks come a few seconds apart).
const musicPageJoin = 30 * time.Second

// pageFlight joins the page lookup running under key, or starts f (not
// tied to the request, up to musicLookupMax). A finished one is kept
// musicPageJoin when it succeeded, so a re-ask never starts a second.
func pageFlight[T any](a *API, ctx context.Context, key string, f func(context.Context) (T, error)) *sideFlight {
	s := &a.side
	s.mu.Lock()
	if fl := s.pages[key]; fl != nil {
		s.mu.Unlock()
		return fl
	}
	if s.pages == nil {
		s.pages = map[string]*sideFlight{}
	}
	fl := &sideFlight{done: make(chan struct{})}
	s.pages[key] = fl
	s.mu.Unlock()
	drop := func() {
		s.mu.Lock()
		if s.pages[key] == fl {
			delete(s.pages, key)
		}
		s.mu.Unlock()
	}
	go func() {
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), musicLookupMax)
		defer cancel()
		v, err := f(lctx)
		fl.v, fl.err = v, err
		close(fl.done)
		// A cut list (one edition of several) is a full answer: kept to join.
		if err != nil && !errors.Is(err, metadata.ErrListCut) {
			drop()
			return
		}
		time.AfterFunc(musicPageJoin, drop)
	}()
	return fl
}

// waitFlight waits for fl until by. answered is false when it was still
// running then (it goes on, and the next ask joins it).
func waitFlight[T any](ctx context.Context, fl *sideFlight, by time.Time) (v T, err error, answered bool) {
	t := time.NewTimer(max(time.Until(by), 0))
	defer t.Stop()
	select {
	case <-fl.done:
		// The value goes with its error: a cut list still has its tracks.
		got, _ := fl.v.(T)
		return got, fl.err, true
	case <-t.C:
		return v, nil, false
	case <-ctx.Done():
		return v, ctx.Err(), false
	}
}

// sideRead answers the kept row of k: memory first, then lookup_cache.
func (a *API) sideRead(ctx context.Context, k string) (sideRow, bool) {
	s := &a.side
	s.mu.Lock()
	row, ok := s.rows[k]
	s.mu.Unlock()
	if ok || a.s.Store == nil {
		return row, ok
	}
	b, ok, err := metadata.LookupCache{Store: a.s.Store}.Get(ctx, musicSidePrefix+k, time.Now())
	if err != nil || !ok || json.Unmarshal(b, &row) != nil || row.At.IsZero() {
		return sideRow{}, false
	}
	a.sideRemember(k, row)
	return row, true
}

func (a *API) sideRemember(k string, row sideRow) {
	s := &a.side
	s.mu.Lock()
	if s.rows == nil || len(s.rows) >= musicSideMax {
		s.rows = map[string]sideRow{}
	}
	s.rows[k] = row
	s.mu.Unlock()
}

// sideKeep stores an answer (or a miss) in memory and in lookup_cache.
func (a *API) sideKeep(k string, v any, found bool) {
	row := sideRow{At: time.Now(), Miss: !found}
	if found {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		row.V = b
	}
	a.sideRemember(k, row)
	if a.s.Store == nil {
		return
	}
	b, err := json.Marshal(row)
	if err != nil {
		return
	}
	until := row.At.Add(musicSideKeep)
	if row.Miss {
		until = row.At.Add(musicSideMiss)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = metadata.LookupCache{Store: a.s.Store}.Put(ctx, musicSidePrefix+k, b, row.At, until)
}

// sideStart runs fetch for k once at a time under base (bounded by
// musicSideFetch) and keeps what it finds.
func sideStart[T any](a *API, base context.Context, k string, fetch func(context.Context) (T, bool, error)) *sideFlight {
	s := &a.side
	s.mu.Lock()
	if f := s.flights[k]; f != nil {
		s.mu.Unlock()
		return f
	}
	if s.flights == nil {
		s.flights = map[string]*sideFlight{}
	}
	f := &sideFlight{done: make(chan struct{})}
	s.flights[k] = f
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.flights, k)
			s.mu.Unlock()
			close(f.done)
		}()
		ctx, cancel := context.WithTimeout(base, musicSideFetch)
		defer cancel()
		v, found, err := fetch(ctx)
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		f.v, f.found, f.err = v, found, err
		if err == nil {
			a.sideKeep(k, v, found)
		}
	}()
	return f
}

// sideGet answers kind/key's side data: kept and fresh at once; kept and
// older than fresh at once, asked again in the background; otherwise asked
// now, waiting no longer than ctx (the lookup goes on and is kept). fetch
// says found=false for nothing known and an error for a failure. ok is
// false when nothing is known.
func sideGet[T any](a *API, ctx context.Context, kind, key string, fresh time.Duration, fetch func(context.Context) (T, bool, error)) (T, bool) {
	var zero T
	k := kind + ":" + key
	if row, ok := a.sideRead(ctx, k); ok {
		age := time.Since(row.At)
		if row.Miss && age < musicSideMiss {
			return zero, false
		}
		var v T
		if !row.Miss && json.Unmarshal(row.V, &v) == nil {
			if age >= fresh {
				sideStart(a, context.Background(), k, fetch)
			}
			return v, true
		}
	}
	f := sideStart(a, context.WithoutCancel(ctx), k, fetch)
	select {
	case <-f.done:
		if v, ok := f.v.(T); ok && f.err == nil {
			return v, f.found
		}
		return zero, false
	case <-ctx.Done():
		return zero, false
	}
}
