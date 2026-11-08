package music

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

// SideRow is one kept answer, in memory and on disk.
type SideRow struct {
	At   time.Time       `json:"at"`
	Miss bool            `json:"miss,omitempty"`
	V    json.RawMessage `json:"v,omitempty"`
}

// SideFlight is one lookup in progress; done closes when it ends.
type SideFlight struct {
	done  chan struct{}
	v     any
	found bool
	err   error
}

// Done closes when the lookup ends.
func (f *SideFlight) Done() <-chan struct{} { return f.done }

type sideStore struct {
	mu      sync.Mutex
	rows    map[string]SideRow
	flights map[string]*SideFlight
	// pages are the artist and album pages' own lookups in progress (and
	// for musicPageJoin after), so a page asked again joins them.
	pages map[string]*SideFlight
}

// musicPageJoin is how long a finished page lookup answers asks again
// (the page's quiet re-asks come a few seconds apart).
const musicPageJoin = 30 * time.Second

// MusicLookupMax bounds a lookup that goes on after the page answered
// (PageFlight).
const MusicLookupMax = time.Minute

// PageFlight joins the page lookup running under key, or starts f (not
// tied to the request, up to MusicLookupMax). A finished one is kept
// musicPageJoin when it succeeded, so a re-ask never starts a second.
func PageFlight[T any](h *Handlers, ctx context.Context, key string, f func(context.Context) (T, error)) *SideFlight {
	s := &h.side
	s.mu.Lock()
	if fl := s.pages[key]; fl != nil {
		s.mu.Unlock()
		return fl
	}
	if s.pages == nil {
		s.pages = map[string]*SideFlight{}
	}
	fl := &SideFlight{done: make(chan struct{})}
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
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), MusicLookupMax)
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

// WaitFlight waits for fl until by. answered is false when it was still
// running then (it goes on, and the next ask joins it).
func WaitFlight[T any](ctx context.Context, fl *SideFlight, by time.Time) (v T, err error, answered bool) {
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

// SideKept is SideRead's kept value for the Library bio (false for a miss).
func (h *Handlers) SideKept(ctx context.Context, k string) (json.RawMessage, bool) {
	row, ok := h.SideRead(ctx, k)
	if !ok || row.Miss {
		return nil, false
	}
	return row.V, true
}

// SideRead answers the kept row of k: memory first, then lookup_cache.
func (h *Handlers) SideRead(ctx context.Context, k string) (SideRow, bool) {
	s := &h.side
	s.mu.Lock()
	row, ok := s.rows[k]
	s.mu.Unlock()
	if ok || h.Store == nil {
		return row, ok
	}
	b, ok, err := metadata.LookupCache{Store: h.Store}.Get(ctx, musicSidePrefix+k, time.Now())
	if err != nil || !ok || json.Unmarshal(b, &row) != nil || row.At.IsZero() {
		return SideRow{}, false
	}
	h.sideRemember(k, row)
	return row, true
}

func (h *Handlers) sideRemember(k string, row SideRow) {
	s := &h.side
	s.mu.Lock()
	if s.rows == nil || len(s.rows) >= musicSideMax {
		s.rows = map[string]SideRow{}
	}
	s.rows[k] = row
	s.mu.Unlock()
}

// sideKeep stores an answer (or a miss) in memory and in lookup_cache.
func (h *Handlers) sideKeep(k string, v any, found bool) {
	row := SideRow{At: time.Now(), Miss: !found}
	if found {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		row.V = b
	}
	h.sideRemember(k, row)
	if h.Store == nil {
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
	_ = metadata.LookupCache{Store: h.Store}.Put(ctx, musicSidePrefix+k, b, row.At, until)
}

// sideStart runs fetch for k once at a time under base (bounded by
// musicSideFetch) and keeps what it finds.
func sideStart[T any](h *Handlers, base context.Context, k string, fetch func(context.Context) (T, bool, error)) *SideFlight {
	s := &h.side
	s.mu.Lock()
	if f := s.flights[k]; f != nil {
		s.mu.Unlock()
		return f
	}
	if s.flights == nil {
		s.flights = map[string]*SideFlight{}
	}
	f := &SideFlight{done: make(chan struct{})}
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
			h.sideKeep(k, v, found)
		}
	}()
	return f
}

// SideGet answers kind/key's side data: kept and fresh at once; kept and
// older than fresh at once, asked again in the background; otherwise asked
// now, waiting no longer than ctx (the lookup goes on and is kept). fetch
// says found=false for nothing known and an error for a failure. ok is
// false when nothing is known.
func SideGet[T any](h *Handlers, ctx context.Context, kind, key string, fresh time.Duration, fetch func(context.Context) (T, bool, error)) (T, bool) {
	var zero T
	k := kind + ":" + key
	if row, ok := h.SideRead(ctx, k); ok {
		age := time.Since(row.At)
		if row.Miss && age < musicSideMiss {
			return zero, false
		}
		var v T
		if !row.Miss && json.Unmarshal(row.V, &v) == nil {
			if age >= fresh {
				sideStart(h, context.Background(), k, fetch)
			}
			return v, true
		}
	}
	f := sideStart(h, context.WithoutCancel(ctx), k, fetch)
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
