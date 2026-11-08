package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// newTrailers builds the metadata trailers on this API's settings, age
// limits, Explore pools and music videos.
func (a *API) newTrailers() *metadata.Trailers {
	t := &metadata.Trailers{}
	t.Store, t.Version = a.s.Store, a.s.Version
	t.Provider = func() metadata.Provider { return a.s.Metadata }
	t.Pick = func() func(ctx context.Context, kind, id, lang string) (string, error) { return a.s.TrailerPick }
	t.Explore, t.Ages, t.Settings, t.People = titleExplore{a}, titleAges{a}, trailerHost{a}, trailerHost{a}
	t.KindsOn, t.Wants, t.Videos = a.kindsOn, trailerHost{a}, a.mh().VideoHooks()
	return t
}

// Trailers is the trailer reader, pass and routes (for the health check
// and the job loops).
func (a *API) Trailers() *metadata.Trailers { return a.trailers }

// trailerHost gives the trailers the look settings, who asks, and the
// titles Explore shows.
type trailerHost struct{ a *API }

func (h trailerHost) AutoplayTrailers(ctx context.Context) bool {
	return health.AutoplayOn(ctx, h.a.s.Store)
}

func (h trailerHost) UserID(r *http.Request) int64 { return h.a.person(r).UserID }

func (h trailerHost) PassTitles(ctx context.Context) [][2]string { return h.a.trailerPassTitles(ctx) }

func (h trailerHost) PassLanguages(ctx context.Context) []string {
	return h.a.trailerPassLanguages(ctx)
}

func (h trailerHost) Warm(lang string, titles [][2]string) { h.a.warmTitles(lang, titles) }

// fillTrailerKeys sets trailer_key on Explore cards whose key is kept
// (discover.FillTrailerKeys).
func (a *API) fillTrailerKeys(r *http.Request, items []discoverItemView) {
	discover.FillTrailerKeys(r, items, a.kidMax(r) != "", a.warmTitles, a.trailers.KnownKey)
}

// trailerPassTitles are the titles the trailer pass fills
// (discover.Handlers.TrailerPassTitles).
func (a *API) trailerPassTitles(ctx context.Context) [][2]string {
	return a.disc.TrailerPassTitles(ctx)
}

// trailerPassLanguages are the languages the trailer pass fills
// (discover.TrailerPassLanguages).
func (a *API) trailerPassLanguages(ctx context.Context) []string {
	return discover.TrailerPassLanguages(ctx, a.s.Store, a.cardArt().LangsAsked())
}

// artistVideosOn says whether artist music videos can play here: music on,
// a music source and a metadata server that lists videos.
func (a *API) artistVideosOn() bool {
	p, _ := parts.ForTitleKind("artist")
	return a.partIsOn(p) && a.s.Library != nil && a.s.Library.Artists != nil && a.s.ArtistVideos != nil
}
