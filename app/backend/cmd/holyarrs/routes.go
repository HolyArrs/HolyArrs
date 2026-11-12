package main

import (
	"context"

	setupmod "github.com/HolyArrs/holyarrs/app/backend/internal/setup"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"

	"github.com/HolyArrs/holyarrs/app/backend/internal/ai"

	"net/http"
	"sync"

	"github.com/HolyArrs/holyarrs/app/backend/internal/music"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// handlers holds every module Handlers the router serves.
type handlers struct {
	s services
	// media serves the media server routes (mediaservers).
	media *mediaservers.Handlers
	// titles serves the extras and wiki routes (metadata).
	titles *metadata.TitlePages
	// sourceH serves the Source, search and bridge routes (modules.go).
	sourceOnce sync.Once
	sourceH    *sources.Handlers
	// dl serves the Downloads routes (internal/downloads).
	dl  *downloads.Web
	dlh *downloads.Handlers
	// mf serves the Library files routes (internal/mediafiles).
	mf *mediafiles.Handlers
	// shows serves the show add, episode, season and anime routes (internal/shows).
	shows *shows.Handlers
	// interest limits each session's live interest calls (webserver.go).
	interest webserver.Interest
	// controls serves the part switches, the controls and the part gate (internal/setup).
	controls *setupmod.Handlers
	// ai serves the AI connections (internal/ai).
	ai *ai.Handlers
	// discover caches Explore rows and details pages and limits each person.
	discover discover.Cache
	// disc serves the Explore rows, details and reviews routes (internal/discover).
	disc *discover.Handlers
	// status keeps the Settings status answer for 30 seconds.
	status health.StatusCache
	// serviceTop keeps the per-service top lists for 12 hours.
	serviceTop discover.ServiceTopCache
	// watchOn keeps each title's TMDB watch providers per region a day.
	watchOn discover.WatchOn
	// mus serves the moved Music code (internal/music); built on first use
	// by mh() so a bare &API{} in tests works too.
	mus     *music.Handlers
	musOnce sync.Once
	// signin keeps the sign-in page's art and its per-address budget.
	signin discover.Signin
	// artMisses is each person's budget of artwork cache misses.
	artMisses discover.ArtworkBudget
	// explore runs the Explore pool job and keeps its lists and pages.
	explore discover.Explore
	// trailers keeps resolved trailer addresses, serves the trailer routes
	// and runs the trailer pass (metadata).
	trailers *metadata.Trailers
	// warm fills listed titles' details, extras and trailers ahead.
	warm    discover.TitleWarmer
	warmSet sync.Once
}

// buildHandlers fills the services, then builds every module Handlers,
// lower layers first, and hands each the funcs it needs from the others.
func buildHandlers(s services) *handlers {
	accounts.SetViewKeyCheck(discover.ExploreViewKeyOK)
	s.fill()
	a := &handlers{s: s, media: &mediaservers.Handlers{Store: s.Store, Vault: s.Secrets, Linker: s.Media}}
	a.mf = &mediafiles.Handlers{Deps: mediafiles.Deps{Store: s.Store, Queue: s.Queue, Metadata: s.Metadata, KidMax: a.kidMax, RouteUsable: routeUsable}}
	a.mf.DetailsRating = a.detailsRating
	a.mf.KindShown = a.kindShown
	a.mf.TitleBatch = discover.WithLookupMemo
	a.mf.AlbumNames = a.albumNames
	a.mf.TitleView = func(ctx context.Context, t mediafiles.Title) (any, error) { return a.titleView(ctx, t) }
	a.mf.CardArt = a.explore.Cards.Library
	a.watchOn.Metadata = s.Metadata
	a.signin.Store, a.signin.Metadata, a.signin.ServeArtwork = s.Store, s.Metadata, library.ServeArtwork
	a.signin.Artwork = func() *library.ArtworkCache {
		if a.s.Library == nil {
			return nil
		}
		return a.s.Library.Artwork
	}
	a.signin.LibraryArtOn = func(ctx context.Context) bool { return health.SigninLibraryArtOn(ctx, a.s.Store) }
	if s.Auth != nil {
		a.signin.ClientKey = s.Auth.ClientKey
	}
	a.wireExplore()
	a.disc = a.newDiscover()
	a.titles = a.newTitlePages()
	a.trailers = a.newTrailers()
	a.ai = &ai.Handlers{Store: s.Store, Version: s.Version, PartOn: a.partIsOn}
	a.shows = shows.NewHandlers(shows.Deps{Store: s.Store, Library: s.Library, Metadata: s.Metadata, AnimeMap: s.AnimeMap,
		PackNext: func(ctx context.Context, titleID int64, season int, by domain.Actor) (int64, int, error) {
			if a.s.Downloads == nil {
				return 0, 0, downloads.ErrNoPack
			}
			return a.s.Downloads.PackNext(ctx, titleID, season, by)
		},
		KidMax: a.kidMax, KidLimited: a.disc.KidLimited, KidShows: a.kidShows, KindsOn: a.disc.KindsOn, LibraryRating: a.libraryRating,
		Caching: a.disc.Caching, CachedDetails: a.disc.LoadedDetails, Details: a.disc.CachedDetails,
		Hidden: func(ctx context.Context, kind, tmdbID string, genres []string) (bool, bool) {
			return discover.HiddenMark(ctx, s.Store, kind, tmdbID, genres)
		},
		OpenAsk: func(r *http.Request, ids map[string]string) (any, error) {
			b, err := a.openShowAsk(r.Context(), ids, a.person(r).UserID)
			if b == nil || err != nil {
				return nil, err
			}
			return b, nil
		},
		Follow: func(r *http.Request, titleID int64) error {
			if a.s.Asks == nil {
				return nil
			}
			return a.s.Asks.FollowAdded(r.Context(), a.person(r), titleID)
		},
		Added: func(ctx context.Context, t mediafiles.Title) (any, error) {
			v, err := a.titleView(ctx, t)
			if err != nil {
				return nil, err
			}
			if a.s.Live != nil {
				a.tv().PublishView(v)
			}
			return v, nil
		},
		KindDefaultsFor: a.kindDefaultsFor, TitleID: a.titleID, StateLabels: StateLabels, Trim: discover.TrimEpisodeSlots, LibraryError: library.WriteError})
	// newTrailers built the music handlers before the trailers and shows existed.
	mh := a.mh()
	mh.Trailers, mh.SlotOfTitle, mh.SeasonLabel = a.trailers, a.shows.SlotOfTitle, a.shows.SeasonLabel
	a.dl = &downloads.Web{Store: s.Store, DLSync: s.DLSync, Downloads: s.Downloads, Live: s.Live, Queue: s.Queue,
		Secrets: s.Secrets, Auth: s.Auth, Library: s.Library, Metadata: s.Metadata, Search: s.Search,
		ShowSource: a.shows.ShowSource, AddMovieTitle: a.addMatchedMovie, AddShowTitle: a.addMatchedShow, LibraryError: library.WriteError}
	a.controls = &setupmod.Handlers{Deps: setupmod.Deps{Store: s.Store, Parts: s.Parts, SafeMode: s.SafeMode, States: s.States, ExitApp: s.ExitApp,
		TitleAllowed: a.titleAllowed, TestDownloader: a.dl.TestDownloader,
		TestSource: func(w http.ResponseWriter, r *http.Request) { a.sourceHandlers().TestSource(w, r) },
		TestSubtitleSite: func(w http.ResponseWriter, r *http.Request) {
			subtitles.NewHandlers(a.s.Store, a.s.Secrets, a.s.Subtitles).ServeSiteTest(w, r)
		}}}
	if s.Media != nil {
		a.controls.TestMediaServer = a.media.Test
	}
	a.dlh = &downloads.Handlers{Deps: downloads.Deps{Store: s.Store, Library: s.Library, Queue: s.Queue, KidMax: a.kidMax,
		TitleID: a.titleID, PublishTitle: a.publishTitle, LibraryError: library.WriteError,
		VersionJSON: func(ctx context.Context, v mediafiles.Version) any {
			return discover.VersionViewOf(ctx, s.Store.Read(), v)
		}}}
	return a
}

// partIsOn reports whether the part p is on (always, without a Control).
// In safe mode the Library still lists every title (parts.SafeLibrary).
func (a *handlers) partIsOn(p parts.Part) bool { return parts.PartOn(a.s.Parts, p) }

// actor is the signed-in person; without sign-in (tests) it is the owner.
func actor(r *http.Request) domain.Actor {
	return webserver.ActorOr(r, domain.Actor{Kind: "user", ID: 1})
}
