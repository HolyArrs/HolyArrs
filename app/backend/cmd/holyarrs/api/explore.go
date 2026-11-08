package api

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// wireExplore points the pool job at this API's store, provider and
// rating lookups (New).
func (a *API) wireExplore() {
	e := &a.explore
	e.Store = func() *database.Store { return a.s.Store }
	e.Metadata = func() metadata.Provider { return a.s.Metadata }
	e.KnownRating = a.knownRating
	e.LookupRating = a.lookupRating
	e.QueueCardArt = a.queueCardArt
}

// StartExplore starts the Explore pool job (discover.Explore) and the
// warmers that go with it.
func (a *API) StartExplore(ctx context.Context) {
	if !a.explore.Start(ctx) {
		return
	}
	a.uiHandlers().ApplyArtworkCap(ctx) // the picture cache size from Settings
	a.s.Store.OnEvent(a.mh().OnEvent)
	// The warmers fetch ahead of anyone: they carry X-Fetch: ahead, keep to
	// the slower pace, wait out the server's hold and have their own
	// breaker, so a restart's warm-up never makes a person's page fail.
	warm := metadata.Ahead(ctx)
	go jobs.Guard("explore-service-top-warm", func() { a.warmServiceTop(warm) })
	go jobs.Guard("explore-genre-warm", func() { a.disc.WarmGenres(warm) })
	discover.StartBackdropDark(ctx)
	a.mh().Start(ctx) // the music sync and catalog bundle warm
	go jobs.Guard("explore-browse-warm", func() { a.disc.WarmBrowse(warm) })
}
