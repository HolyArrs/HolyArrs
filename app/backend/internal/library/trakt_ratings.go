package library

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Trakt ratings are read by a slow pass of their own, never by the details
// refresh, so Trakt being slow or busy cannot hold it up.
const (
	// TraktEvery is how often the pass runs; with traktPerPass that is at
	// most 2,400 requests a day, far under Trakt's limit of 1,000 every 5
	// minutes.
	TraktEvery   = 30 * time.Minute
	traktPerPass = 50
	// traktFresh is how long a title's rating is kept before Trakt is asked again.
	traktFresh = 7 * 24 * time.Hour
	// traktBackOff is how long the pass rests after Trakt refused the
	// client id; a new client id ends the rest. A "slow down" rests it for
	// traktSlowDown.
	traktBackOff  = 12 * time.Hour
	traktSlowDown = time.Hour
)

// traktState is the pass's rest.
type traktState struct {
	mu    sync.Mutex
	until time.Time
	key   string // the client id the rest is for
}

// TraktPass reads the Trakt ratings of up to traktPerPass Library movies and
// shows that have none yet or whose rating is over a week old. Without a
// Trakt client id it does nothing. Errors are logged, never returned: a
// pass that cannot reach Trakt simply tries again next time.
func (s *Service) TraktPass(ctx context.Context) error {
	key := s.TraktRatings.KeyNow(ctx)
	if key == "" {
		return nil
	}
	now := s.now()
	st := &s.trakt
	st.mu.Lock()
	resting := now.Before(st.until) && st.key == key
	st.mu.Unlock()
	if resting {
		return nil
	}
	rest := func(d time.Duration) {
		st.mu.Lock()
		st.until, st.key = s.now().Add(d), key
		st.mu.Unlock()
	}
	due, err := mediafiles.TitlesDueForTrakt(ctx, s.Store.Read(), now.Add(-traktFresh), traktPerPass)
	if err != nil {
		log.Printf("library: Trakt ratings: %v", err)
		return nil
	}
	for _, d := range due {
		if ctx.Err() != nil {
			return nil
		}
		sc, err := s.TraktRatings.Rating(ctx, d.Kind, d.IMDb)
		switch {
		case errors.Is(err, metadata.ErrTraktLocked):
			rest(traktBackOff)
			log.Printf("library: Trakt refused the client id; trying again in %s", traktBackOff)
			return nil
		case errors.Is(err, metadata.ErrRateLimited):
			rest(traktSlowDown)
			return nil
		case errors.Is(err, metadata.ErrNotFound):
			err = mediafiles.SaveTraktRating(ctx, s.Store, d.ID, 0, 0, false, s.now())
		case err != nil:
			log.Printf("library: Trakt rating for title %d: %v", d.ID, err)
			return nil
		default:
			err = mediafiles.SaveTraktRating(ctx, s.Store, d.ID, sc.Rating, sc.Votes, true, s.now())
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("library: Trakt rating for title %d: %v", d.ID, err)
		}
	}
	return nil
}
