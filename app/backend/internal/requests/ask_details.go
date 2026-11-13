package requests

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// AskDetails are the name, year and poster a request card shows.
type AskDetails struct {
	Name   string
	Year   int
	Poster string
}

// LookupAskDetails finds a movie's or show's details by its ref
// ("tmdb:603", "tvdb:81189"), the way a request made in HolyArrs gets them.
func LookupAskDetails(ctx context.Context, lib *library.Service, kind, ref string) (AskDetails, error) {
	if kind == "movie" {
		m, err := lib.Metadata.Movie(ctx, ref)
		return AskDetails{Name: m.Name, Year: m.Year, Poster: m.Poster}, err
	}
	defs, err := library.ReadShowDefaults(ctx, lib.Store.Read())
	if err != nil {
		return AskDetails{}, err
	}
	sh, err := shows.New(lib).LookupShow(ctx, ref, defs.Source)
	return AskDetails{Name: sh.Name, Year: sh.Year, Poster: sh.Poster}, err
}

// PlaceholderName is the stand-in name an imported request gets while its
// real name is not known: the id it is found by ("TVDB 81189").
func PlaceholderName(ref string) string {
	i := strings.IndexByte(ref, ':')
	if i < 0 {
		return ref
	}
	return strings.ToUpper(ref[:i]) + " " + ref[i+1:]
}

// isPlaceholder says whether name is empty or a stand-in for one of ids.
func isPlaceholder(name, ref string, ids map[string]string) bool {
	if name == "" || name == PlaceholderName(ref) {
		return true
	}
	for k, v := range ids {
		if name == PlaceholderName(k+":"+v) {
			return true
		}
	}
	return false
}

// How often waiting fill-ins are tried again, and how many per pass.
const (
	fillEvery   = 10 * time.Minute
	fillPerPass = 50
)

// askFill keeps the fill-in to one pass at a time and remembers the asks
// already filled since start.
type askFill struct {
	sync.Mutex
	busy  bool
	next  time.Time
	after int64
	done  map[int64]bool
}

// FillAskDetailsLater starts a fill-in pass in the background when one is
// due (it is cheap to call often).
func (s *Service) FillAskDetailsLater() {
	s.fill.Lock()
	now := time.Now()
	if s.fill.busy || now.Before(s.fill.next) {
		s.fill.Unlock()
		return
	}
	s.fill.busy, s.fill.next = true, now.Add(fillEvery)
	s.fill.Unlock()
	go func() {
		defer func() { s.fill.Lock(); s.fill.busy = false; s.fill.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if _, err := s.FillAskDetails(ctx); err != nil {
			log.Printf("asks: fill in names: %v", err)
		}
	}()
}

// FillAskDetails gives movie and show asks still without their real name,
// year or poster (requests brought over from another app) what the details
// source knows. An ask whose lookup fails keeps what it has and is tried
// again on a later pass. It returns how many asks were filled.
func (s *Service) FillAskDetails(ctx context.Context) (int, error) {
	if s.Library == nil || s.Library.Metadata == nil {
		return 0, nil
	}
	ids, err := fillCandidates(ctx, s.Store.Read())
	if err != nil {
		return 0, err
	}
	// Each pass starts after the last ask the one before tried, so asks
	// that keep failing never hold back the ones after them.
	s.fill.Lock()
	after := s.fill.after
	s.fill.Unlock()
	for i, id := range ids {
		if id > after {
			ids = append(append([]int64{}, ids[i:]...), ids[:i]...)
			break
		}
	}
	n, tried := 0, 0
	for _, id := range ids {
		s.fill.Lock()
		skip := s.fill.done[id]
		s.fill.Unlock()
		if skip {
			continue
		}
		if tried++; tried > fillPerPass || ctx.Err() != nil {
			break
		}
		s.fill.Lock()
		s.fill.after = id
		s.fill.Unlock()
		a, err := get(ctx, s.Store.Read(), id)
		if err != nil || a.Ref == "" {
			continue
		}
		placeholder := isPlaceholder(a.Name, a.Ref, a.SourceIDs)
		if !placeholder && (a.Poster != "" || a.Year != nil) {
			continue
		}
		lctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		d, err := LookupAskDetails(lctx, s.Library, a.Kind, a.Ref)
		cancel()
		if err != nil || d.Name == "" {
			log.Printf("asks: details of ask %d (%s): %v", a.ID, a.Ref, err)
			continue
		}
		name, year, poster := a.Name, a.Year, a.Poster
		if placeholder {
			name = d.Name
		}
		if year == nil && d.Year > 0 {
			y := d.Year
			year = &y
		}
		if poster == "" {
			poster = d.Poster
		}
		if err := setAskDetails(ctx, s.Store, a.ID, a.Name, name, year, poster, s.now()); err != nil {
			return n, err
		}
		s.fill.Lock()
		if s.fill.done == nil {
			s.fill.done = map[int64]bool{}
		}
		s.fill.done[a.ID] = true
		s.fill.Unlock()
		n++
		s.changed(ctx, a.ID)
	}
	return n, nil
}
