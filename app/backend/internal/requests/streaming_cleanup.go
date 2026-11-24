package requests

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// cleanupReaders is how many watch provider reads a library check runs at
// once; cleanupLimit how long one whole check may take.
const (
	cleanupReaders = 4
	cleanupLimit   = 2 * time.Hour
)

// OnServicesTitle is a title with files that streams on one of the
// owner's services: a suggestion to remove, never removed here.
type OnServicesTitle struct {
	mediafiles.TitleOnDisk
	Service string `json:"service"`
}

// CleanupCheck is the state of the latest library check: titles with
// files that stream on the owner's services.
type CleanupCheck struct {
	Running  bool              `json:"running"`
	Checked  int               `json:"checked"`
	Total    int               `json:"total"`
	Failed   int               `json:"failed"`
	Region   string            `json:"region"`
	Found    []OnServicesTitle `json:"found"`
	Started  *time.Time        `json:"started_at,omitempty"`
	Finished *time.Time        `json:"finished_at,omitempty"`
}

// streamingCleanup keeps the latest check in memory; a restart forgets it.
type streamingCleanup struct {
	mu  sync.Mutex
	cur CleanupCheck
}

// CleanupState answers the latest library check.
func (s *Service) CleanupState() CleanupCheck {
	s.cleanup.mu.Lock()
	defer s.cleanup.mu.Unlock()
	c := s.cleanup.cur
	c.Found = append([]OnServicesTitle{}, c.Found...)
	return c
}

// StartCleanup starts a library check in the background unless one runs.
// It answers ErrStreamingRegion or ErrNoMyServices when My streaming
// services are not set.
func (s *Service) StartCleanup(ctx context.Context) (CleanupCheck, error) {
	set, err := ReadSettings(ctx, s.Store.Read())
	if err != nil {
		return CleanupCheck{}, err
	}
	if set.StreamingRegion == "" {
		return CleanupCheck{}, ErrStreamingRegion
	}
	if len(set.StreamingServices) == 0 {
		return CleanupCheck{}, ErrNoMyServices
	}
	titles, err := mediafiles.TitlesOnDisk(ctx, s.Store.Read())
	if err != nil {
		return CleanupCheck{}, err
	}
	s.cleanup.mu.Lock()
	if s.cleanup.cur.Running {
		s.cleanup.mu.Unlock()
		return s.CleanupState(), nil
	}
	now := s.now()
	s.cleanup.cur = CleanupCheck{Running: true, Total: len(titles), Region: set.StreamingRegion, Found: []OnServicesTitle{}, Started: &now}
	s.cleanup.mu.Unlock()
	done := make(chan struct{})
	go jobs.Guard("streaming-cleanup", func() {
		defer close(done)
		s.runCleanup(titles, set)
	})
	if s.CleanupWait {
		<-done
	}
	return s.CleanupState(), nil
}

// runCleanup reads each title's watch providers, a few at a time.
func (s *Service) runCleanup(titles []mediafiles.TitleOnDisk, set Settings) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupLimit)
	defer cancel()
	work := make(chan mediafiles.TitleOnDisk)
	var wg sync.WaitGroup
	for range cleanupReaders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range work {
				got, err := s.watchOf(ctx, t.Kind, t.TMDBID)
				sv, on := OnMyServices(got, set.StreamingRegion, set.StreamingServices)
				s.cleanup.mu.Lock()
				s.cleanup.cur.Checked++
				if err != nil {
					s.cleanup.cur.Failed++
				} else if on {
					s.cleanup.cur.Found = append(s.cleanup.cur.Found, OnServicesTitle{TitleOnDisk: t, Service: sv.Name})
				}
				s.cleanup.mu.Unlock()
			}
		}()
	}
	for _, t := range titles {
		if ctx.Err() != nil {
			break
		}
		work <- t
	}
	close(work)
	wg.Wait()
	s.cleanup.mu.Lock()
	defer s.cleanup.mu.Unlock()
	// Biggest first, as listed: the readers finish out of order.
	order := make(map[int64]int, len(titles))
	for i, t := range titles {
		order[t.ID] = i
	}
	f := s.cleanup.cur.Found
	sort.SliceStable(f, func(i, j int) bool { return order[f[i].ID] < order[f[j].ID] })
	now := s.now()
	s.cleanup.cur.Running, s.cleanup.cur.Finished = false, &now
}
