package api

import (
	"context"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// The Kid limit lives in internal/mediafiles; these wrappers keep the
// callers still in api unchanged until they move.

// aboveLimit is the answer to a TMDB title above the limit.
const aboveLimit = metadata.AboveLimit

type ratingRef = mediafiles.RatingRef

// kidMax is the signed-in person's age-rating limit: "" for no limit, for
// owners and admins, and without sign-in.
func (a *API) kidMax(r *http.Request) string { return accounts.KidMax(r) }

// detailsRating is the rating a cached details page already knows.
func (a *API) detailsRating(kind, tmdbID string) (string, bool) {
	if e, ok := a.discover.Get("details|" + kind + "|" + tmdbID); ok && e.Details != nil && time.Since(e.At) < discover.TTL["details"] {
		return e.Details.Rating, true
	}
	return "", false
}

// routeUsable says the signed-in person may use the route pattern.
func routeUsable(r *http.Request, pattern string) bool {
	who, _ := webserver.From(r.Context())
	need, known := accounts.RoutePerms[pattern]
	return known && who.Has(need) && who.HasScope(accounts.RouteScope(pattern))
}

func (a *API) titleAllowed(r *http.Request, t mediafiles.Title) bool { return a.mf.TitleAllowed(r, t) }

func (a *API) libraryRating(ctx context.Context, kind, tmdbID string) (string, bool) {
	return a.mf.LibraryRating(ctx, kind, tmdbID)
}

func (a *API) knownRating(ctx context.Context, ref ratingRef) (string, bool) {
	return a.mf.KnownRating(ctx, ref)
}

func (a *API) lookupRating(ctx context.Context, ref ratingRef) (string, bool) {
	return a.mf.LookupRating(ctx, ref)
}

func (a *API) kidAllowed(ctx context.Context, max string, refs []ratingRef, n int) []bool {
	return a.mf.KidAllowed(ctx, max, refs, n)
}

func (a *API) kidItems(ctx context.Context, max string, items []metadata.DiscoverItem, n int) []metadata.DiscoverItem {
	return a.mf.KidItems(ctx, max, items, n)
}

func (a *API) kidMovies(ctx context.Context, max string, ms []metadata.Movie) []metadata.Movie {
	return a.mf.KidMovies(ctx, max, ms)
}

func (a *API) kidShows(ctx context.Context, max string, shows []metadata.Show) []metadata.Show {
	return a.mf.KidShows(ctx, max, shows)
}

func (a *API) libraryMovieAllowed(ctx context.Context, max, tmdbID string) bool {
	return a.mf.LibraryMovieAllowed(ctx, max, tmdbID)
}

func (a *API) kidKnownFor(ctx context.Context, max string, p *metadata.Person) {
	a.mf.KidKnownFor(ctx, max, p)
}

func (a *API) allowedTitleIDs(ctx context.Context, max string, ids []int64) (map[int64]bool, error) {
	return a.mf.AllowedTitleIDs(ctx, max, ids)
}

func (a *API) kidGate(m *http.ServeMux, next http.Handler) http.Handler { return a.mf.KidGate(m, next) }

func kidSQL(max string) (string, []any) { return mediafiles.KidSQL(max) }

// kindShown says the Library summary and search show a title kind: its
// part is on, or safe mode shows every part.
func (a *API) kindShown(kind string) bool {
	part, ok := parts.ForTitleKind(kind)
	return !ok || a.partIsOn(part) || (a.s.Parts != nil && a.s.Parts.SafeMode)
}

// albumNames hands the albums a Library search matched to the music
// picture chain and prefetch, as Explore's rows do.
func (a *API) albumNames(list []mediafiles.AlbumName) {
	names := make([]music.NameKey, 0, len(list))
	for _, n := range list {
		names = append(names, music.NewNameKey("album", n.MBID, n.Name, n.Artist))
	}
	a.mh().RememberNames(names)
	a.mh().WarmNames(names)
}

func (a *API) libraryPeople(r *http.Request, words []string, visible func(mediafiles.Title) bool) ([]mediafiles.LibraryPerson, error) {
	return a.mf.LibraryPeople(r, words, visible)
}

func fold(s string) string                    { return mediafiles.Fold(s) }
func foldWords(s string) []string             { return mediafiles.FoldWords(s) }
func foldMatch(s string, words []string) bool { return mediafiles.FoldMatch(s, words) }
