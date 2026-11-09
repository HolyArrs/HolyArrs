package shows

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The library runs show Titles through these steps.
func init() {
	library.UseShows(func(lib *library.Service) library.ShowParts { return New(lib) })
}

var _ library.ShowParts = (*Service)(nil)

// Refresh is RefreshShow in the shape the shared title refresh reports.
func (s *Service) Refresh(ctx context.Context, titleID int64, by domain.Actor, jobID *int64) (library.RefreshOutcome, error) {
	r, err := s.RefreshShow(ctx, titleID, by, jobID)
	return library.RefreshOutcome{Dropped: r.Dropped, GateDriver: "air_date", Opened: r.Opened, Closed: r.Closed}, err
}

// PlanAdopt lists a show Library folder's folders and their matches.
func (s *Service) PlanAdopt(ctx context.Context, folderID int64, after string) (library.AdoptPlan, error) {
	return PlanShowAdoptFrom(ctx, s.Store.Read(), s.Shows, folderID, after)
}

// AdoptOne adds one confirmed show folder.
func (s *Service) AdoptOne(ctx context.Context, lf mediafiles.LibraryFolder, c library.AdoptChoice, by domain.Actor) (library.AdoptResult, error) {
	return s.adoptShowOne(ctx, lf, c, by)
}

// RecordFound records episode files found in a show Version's folder.
func (s *Service) RecordFound(ctx context.Context, lf mediafiles.LibraryFolder, v mediafiles.Version, known map[string]bool, by domain.Actor) (int, error) {
	return s.recordFoundEpisodes(ctx, lf, v, known, by)
}

// SearchShow searches every wanted season of a new show.
func (s *Service) SearchShow(ctx context.Context, titleID int64, by domain.Actor) error {
	return s.searchShow(ctx, titleID, by)
}

// RegisterJobs adds the show Jobs to the queue.
func (s *Service) RegisterJobs(q *jobs.Queue) {
	q.Handle(KindRefreshShows, s.handleRefreshShows)
}
