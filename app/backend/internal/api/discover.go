package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type discoverEntry = discover.Entry

// discoverState holds the row cache and each person's request times.
type discoverState = discover.Cache

// discoverItemView is one discover item with what the Library knows of it.
type discoverItemView = discover.ItemView

// The wrappers below keep api callers on the moved discover handlers until
// their own files move.

func (a *API) loadedDetails(kind, id string) *metadata.Details { return a.disc.LoadedDetails(kind, id) }

func (a *API) kidLimited(w http.ResponseWriter, r *http.Request) bool { return a.disc.KidLimited(w, r) }

func (a *API) exploreCaching(w http.ResponseWriter, r *http.Request, cache string) {
	a.disc.Caching(w, r, cache)
}

func (a *API) kindsOn(kind string) []string { return a.disc.KindsOn(kind) }

func (a *API) cachedRow(ctx context.Context, d metadata.Discoverer, row, kind, ref, genre string) ([]metadata.DiscoverItem, error) {
	return a.disc.CachedRow(ctx, d, row, kind, ref, genre)
}

func (a *API) libraryJoin(ctx context.Context, it metadata.DiscoverItem, me int64) (discoverItemView, bool, error) {
	return a.disc.LibraryJoin(ctx, it, me)
}

func (a *API) dropNeverAdd(ctx context.Context, items []metadata.DiscoverItem) ([]metadata.DiscoverItem, error) {
	return a.disc.DropNeverAdd(ctx, items)
}

func (a *API) joinItems(ctx context.Context, items []metadata.DiscoverItem, me int64, n int) ([]discoverItemView, error) {
	return a.disc.JoinItems(ctx, items, me, n)
}

func (a *API) cachedDetails(ctx context.Context, d metadata.Discoverer, kind, id string) (*metadata.Details, error) {
	return a.disc.CachedDetails(ctx, d, kind, id)
}

// newDiscover builds the Explore handlers on this API's cache and glue.
func (a *API) newDiscover() *discover.Handlers {
	return &discover.Handlers{Deps: discover.Deps{Store: a.s.Store, Metadata: a.s.Metadata, Secrets: a.s.Secrets, TraktBase: a.s.TraktBase,
		Cache: &a.discover, Explore: &a.explore, KidMax: a.kidMax, UserID: func(r *http.Request) int64 { return a.person(r).UserID }, Actor: actor,
		PartOn: a.partIsOn, Parts: a.s.Parts, TitleAllowed: a.titleAllowed, LibraryRating: a.libraryRating, KidItems: a.kidItems,
		CardFresh: a.cardFresh, WithFreshRow: withFreshRow, OpenAsk: a.openAsk, OpenShowAsk: a.openShowAsk,
		FillCardArt: a.fillCardArt, QueueCardItems: a.queueCardItems,
		ServiceTop: func() discover.ServiceTopFetch { return a.s.ServiceTop }, ServiceTopCache: &a.serviceTop, WatchOn: &a.watchOn, FillTrailerKeys: a.fillTrailerKeys,
		KidKnownFor: a.kidKnownFor, LibraryMovieAllowed: a.libraryMovieAllowed,
		LiveSource: func(ctx context.Context) discover.Lister { return a.liveSource(ctx) },
		TitleView:  a.titleView, KidMovies: a.kidMovies,
		AllowedTitleIDs: a.allowedTitleIDs, KnownGenres: func(mbid string) []string { return a.mh().KnownGenres(mbid) }, ReadCardArt: a.readCardArt,
		GenreLists: a.genreLists, NoteTrailers: func(lang string, titles [][2]string) { a.trailers.Note(lang, titles) },
		WarmMusicNew: func() { a.musicProvider().WarmNew() },
		Strip: a.stripBrowse()}}
}

func peopleError(w http.ResponseWriter, err error) { discover.PeopleError(w, err) }

// discoverCache is the Explore row and details cache (for tests, so they
// keep working when the cache moves into the discover module).
func (a *API) discoverCache() *discover.Cache { return &a.discover }

// exploreRowView is one row of the head or tail (discover.ExploreRowView).
type exploreRowView = discover.ExploreRowView

func (a *API) explorePageFor(ctx context.Context, r *http.Request, member int64, sw string) (engine.Page, bool, error) {
	return a.disc.ExplorePageFor(ctx, r, member, sw)
}

func (a *API) poolPage(p engine.Pool, member int64, sw, region string, m engine.Moment) engine.Page {
	return a.disc.PoolPage(p, member, sw, region, m)
}

func (a *API) exploreViews(ctx context.Context, r *http.Request, rows []engine.Row) ([]exploreRowView, error) {
	return a.disc.ExploreViews(ctx, r, rows)
}
