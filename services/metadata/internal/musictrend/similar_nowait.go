package musictrend

import (
	"context"
	"strings"
	"time"
)

// similarBackground caps the similar-artists fetches running for page
// opens at once; similarWaiting caps the misses waiting for one of those
// slots. A miss past both answers ErrPending and is started on the next
// ask.
var (
	similarBackground = make(chan struct{}, 8)
	similarWaiting    = make(chan struct{}, 256)
)

// ArtistsNow answers from what is kept and never waits on a source: a
// fresh answer is a "hit", an expired one is served as "stale" while it is
// fetched again, and a miss answers ErrPending while the fetch runs in the
// background (the next ask finds it). The background flight is registered
// under the lock, so two asks never start it twice; when every slot is
// busy it waits for one instead of never starting.
func (s *Similar) ArtistsNow(ctx context.Context, mbid string) ([]byte, string, error) {
	mbid = strings.ToLower(mbid)
	if !validMBID(mbid) {
		return nil, "", ErrNoSimilar
	}
	s.mu.Lock()
	if s.entries == nil {
		s.entries, s.flights = map[string]similarEntry{}, map[string]*similarFlight{}
	}
	old, had := s.entries[mbid]
	if !had {
		old, had = s.loadSimilarLocked(mbid)
	}
	fresh := had && s.now().Sub(old.at) < s.ttl(old)
	if !fresh {
		if _, running := s.flights[mbid]; !running {
			select {
			case similarWaiting <- struct{}{}:
				f := &similarFlight{done: make(chan struct{})}
				s.flights[mbid] = f
				go s.background(ctx, mbid, f, old, had)
			default:
			}
		}
	}
	s.mu.Unlock()
	if fresh {
		return old.body, "hit", old.err
	}
	if had && old.err == nil && old.body != nil {
		return old.body, "stale", nil
	}
	return nil, "miss", ErrPending
}

// background waits for a fetch slot and runs the registered flight; a
// flight that never gets a slot within its time ends with nothing kept.
func (s *Similar) background(ctx context.Context, mbid string, f *similarFlight, old similarEntry, had bool) {
	defer func() { <-similarWaiting }()
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	select {
	case similarBackground <- struct{}{}:
	case <-bg.Done():
		s.mu.Lock()
		delete(s.flights, mbid)
		s.mu.Unlock()
		f.e = similarEntry{err: bg.Err(), at: s.now()}
		close(f.done)
		return
	}
	defer func() { <-similarBackground }()
	_, _, _ = s.fly(bg, mbid, f, old, had)
}
