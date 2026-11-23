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

// OMDb scores are read by a slow pass of their own, never by the details
// refresh, so OMDb being slow, down or out of requests cannot hold it up.
const (
	// OMDbEvery is how often the pass runs. With omdbPerPass that is at
	// most 960 requests a day, under the free key's 1,000.
	OMDbEvery   = 30 * time.Minute
	omdbPerPass = 20
	// omdbPerDay caps a day's requests even when restarts run extra passes.
	omdbPerDay = 950
	// omdbFresh is how long a title's scores are kept before OMDb is asked again.
	omdbFresh = 7 * 24 * time.Hour
	// omdbBackOff is how long the pass rests after OMDb refused the key or
	// said the day's requests are used up. A new key ends the rest.
	omdbBackOff = 12 * time.Hour
)

// omdbState is the pass's rest and its count of today's requests.
type omdbState struct {
	mu    sync.Mutex
	until time.Time
	key   string // the key that was refused
	day   string
	count int
}

// OMDbPass reads the scores of up to omdbPerPass Library movies and shows
// that have none yet or whose scores are over a week old. Without an OMDb
// key it does nothing. Errors are logged, never returned: a pass that
// cannot reach OMDb simply tries again next time.
func (s *Service) OMDbPass(ctx context.Context) error {
	if s.OMDb == nil {
		return nil
	}
	key := s.OMDb.KeyNow(ctx)
	if key == "" {
		return nil
	}
	now := s.now()
	st := &s.omdb
	st.mu.Lock()
	resting := now.Before(st.until) && st.key == key
	if day := now.Format("2006-01-02"); st.day != day {
		st.day, st.count = day, 0
	}
	room := min(omdbPerPass, omdbPerDay-st.count)
	st.mu.Unlock()
	if resting || room <= 0 {
		return nil
	}
	due, err := mediafiles.TitlesDueForOMDb(ctx, s.Store.Read(), now.Add(-omdbFresh), room)
	if err != nil {
		log.Printf("library: OMDb scores: %v", err)
		return nil
	}
	for _, d := range due {
		if ctx.Err() != nil {
			return nil
		}
		st.mu.Lock()
		st.count++
		st.mu.Unlock()
		sc, err := s.OMDb.Scores(ctx, d.IMDb)
		switch {
		case errors.Is(err, metadata.ErrNoAccess):
			st.mu.Lock()
			st.until, st.key = s.now().Add(omdbBackOff), key
			st.mu.Unlock()
			log.Printf("library: OMDb refused the key or the day's requests are used up; trying again in %s", omdbBackOff)
			return nil
		case errors.Is(err, metadata.ErrSourceOff):
			return nil // lookups are off
		case errors.Is(err, metadata.ErrNotFound):
			err = mediafiles.SaveOMDbScores(ctx, s.Store, d.ID, mediafiles.OutsideScores{}, false, s.now())
		case err != nil:
			log.Printf("library: OMDb scores for title %d: %v", d.ID, err)
			return nil
		default:
			err = mediafiles.SaveOMDbScores(ctx, s.Store, d.ID, mediafiles.OutsideScores{IMDbRating: sc.IMDbRating, IMDbVotes: sc.IMDbVotes,
				RottenTomatoes: sc.RottenTomatoes, Metacritic: sc.Metacritic}, true, s.now())
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("library: OMDb scores for title %d: %v", d.ID, err)
		}
	}
	return nil
}
