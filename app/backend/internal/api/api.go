// Package api serves the versioned HTTP API under /api/v1, the live stream
// at /live and the web app at /.
package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/HolyArrs/holyarrs/app/backend/internal/datapack"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	setupmod "github.com/HolyArrs/holyarrs/app/backend/internal/setup"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/movies"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/titleview"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"

	"github.com/HolyArrs/holyarrs/app/backend/internal/ai"

	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/music"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/features"
)

// Server is what the API needs.
type Server struct {
	Store *database.Store
	// DLSync is the background download reader (downloads.sync = on); nil
	// keeps the Downloads view's own client reads.
	DLSync   *downloads.Sync
	Queue    *jobs.Queue
	Live     *webserver.LiveHub
	Metadata metadata.Provider
	Version  string
	Started  time.Time
	// Auth guards every route but the health check and the sign-in page.
	// Nil serves without sign-in (tests only).
	Auth *accounts.Auth
	// Backups serves the backup routes when set; Restart is called after a
	// restore is staged.
	Backups *backups.Manager
	Restart func()
	// Secrets stores the encrypted settings.
	Secrets *secrets.Vault
	// Bridges keeps the connected Jackett and Prowlarr; nil makes one.
	Bridges *sources.BridgeService
	// TraktBase is the Trakt API address for reviews; empty is the real one.
	TraktBase string
	// MetadataUp reports whether the metadata server answers (cached
	// about a minute); nil counts as up.
	MetadataUp func(context.Context) bool
	// ServiceTop reads every streaming service's top list for a region
	// from the metadata server; nil leaves the row empty.
	ServiceTop func(ctx context.Context, region string) (metadata.ServiceTop, error)
	// TrailerPick reads the metadata server's trailer key for a title
	// (kind movie or show); nil uses only TMDB's videos.
	TrailerPick func(ctx context.Context, kind, id, lang string) (string, error)
	// AnimeMap reads the metadata server's anime ID mapping (by tmdb_tv,
	// tmdb_movie, tvdb, anilist or mal); nil maps nothing.
	AnimeMap func(ctx context.Context, by, id string) (metadata.AnimeMap, error)
	// Deps are the metadata server calls and flags the music code reads
	// (internal/music).
	music.Deps
	// LiveSearch is flag live_search (music plan Phase 4): the search
	// field's dropdown, GET /api/v1/search/suggest; off answers 404 and
	// the field shows no dropdown.
	LiveSearch bool
	// ExtraArt and ExtraRatings read the server's stored title art and outside ratings
	// from the metadata server; nil (or a source off there) shows neither.
	ExtraArt func(ctx context.Context, kind, id string) (metadata.ServerArt, error)
	// ImageBase is the metadata server's picture origin ("<server>/v1/img");
	// its sharp artist photo is the first choice for every artist picture.
	// Empty skips it.
	ImageBase    string
	ExtraRatings func(ctx context.Context, kind, id string) (metadata.ServerRatings, error)
	// WikiTitle and WikiPerson read awards and the Wikipedia bio from the
	// metadata server; AwardWinners its award-winning TMDB IDs (the Award
	// winners collection). nil hides them.
	WikiTitle    func(ctx context.Context, kind, id string) (metadata.WikiEntity, error)
	WikiPerson   func(ctx context.Context, id, name string) (metadata.WikiEntity, error)
	AwardWinners func(ctx context.Context, kind string) ([]metadata.AwardWinner, error)
	// MusicControl reads the metadata server's switch for fetch-ahead
	// (on, slow, off); ArtistVideos an artist's official music videos.
	// nil, or an older server without them, keeps fetch-ahead on and the
	// artist hero a photo.
	MusicControl func(ctx context.Context) (metadata.Control, error)
	ArtistVideos func(ctx context.Context, mbid string) ([]metadata.ArtistVideo, error)
	// CountryCount passes "Count this install" (Settings, This app) on to
	// the requests to the metadata server (metadata.Gate.SetCountryCount).
	// nil: nothing to tell.
	CountryCount func(on bool)
	// Packages is the local copy of the metadata server's artist packages
	// (internal/music): music pages answer from it first. nil: none.
	Packages *music.PackageSync
	// Library runs adding, editing and refreshing; New makes one when nil.
	Library *library.Service
	// Lists reads Outside lists; New makes one without keys when nil.
	Lists *requests.OutsideLists
	// Alerts sends Discord and email Alerts; New makes one when Secrets is set.
	Alerts *alerts.Sender
	// Asks runs Requests; New makes one when nil.
	Asks *requests.Service
	// Watchlists reads Plex watchlists (nil: no person can be read).
	Watchlists *requests.Watchlists
	// Downloads follows Downloaders and Downloads; made on first use when nil.
	Downloads *downloads.Manager
	// Search runs searches, hand choices and the Held and Turned down
	// lists; New makes one (choices queue a grab Job) when nil.
	Search *sources.Searcher
	// Health runs the health checks; New makes one when nil.
	Health *health.Engine
	// Updates is the update check; New makes one when nil and hands it to
	// Health when Health has no update check yet.
	Updates *updates.Checker
	// Media tells media servers about changed folders; New makes one when
	// Secrets is set.
	Media *mediaservers.Linker
	// Subtitles runs subtitle work; New makes one when nil.
	Subtitles *subtitles.Service
	// States holds the state of every supervised stage (may be nil).
	States *jobs.Registry
	// SafeMode says the app started with only the core.
	SafeMode bool
	// Parts switches the five parts and the stages (nil: every part on,
	// no controls).
	Parts *parts.Control
	// Mover runs moves from the old apps; made on first use when nil.
	Mover *migrate.Mover
	// ExitApp stops the process for a whole-app restart ("restart"), a
	// restart in safe mode ("safe_mode") or a shut down ("shutdown"); nil
	// refuses those controls. It returns an error, and stops nothing,
	// when it cannot arrange the exit. Tests record the call instead.
	ExitApp func(how string) error
	// Log is the app log for the log viewer and download (nil: none kept).
	Log *logs.Log
	// ConfigDir and SavePath are shown on This app (ConfigDir defaults to
	// the database folder).
	ConfigDir string
	SavePath  string
	// URLBase is the path HolyArrs runs under behind a proxy ("" for none),
	// shown on This app.
	URLBase string
}

// API serves HTTP.
type API struct {
	s Server
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
	interest interestState
	// controls serves the part switches, the controls and the part gate (internal/setup).
	controls *setupmod.Handlers
	// ai serves the AI connections (internal/ai).
	ai *ai.Handlers
	// discover caches Explore rows and details pages and limits each person.
	discover discoverState
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
	explore   exploreState
	genreRows genreRowCache
	// trailers keeps resolved trailer addresses, serves the trailer routes
	// and runs the trailer pass (metadata).
	trailers *metadata.Trailers
	// warm fills listed titles' details, extras and trailers ahead.
	warm titleWarmer
}

// New makes the API.
func New(s Server) *API {
	accounts.SetViewKeyCheck(discover.ExploreViewKeyOK)
	if s.Started.IsZero() {
		s.Started = time.Now()
	}
	if s.Library == nil {
		s.Library = &library.Service{Store: s.Store, Queue: s.Queue, Metadata: s.Metadata, Live: s.Live}
	}
	if s.Lists == nil {
		s.Lists = &requests.OutsideLists{Store: s.Store, Queue: s.Queue, Library: s.Library, Metadata: s.Metadata}
	}
	if s.Alerts == nil && s.Secrets != nil {
		s.Alerts = &alerts.Sender{Store: s.Store, Vault: s.Secrets}
	}
	if s.Asks == nil {
		s.Asks = &requests.Service{Store: s.Store, Library: s.Library, Metadata: s.Metadata, Live: s.Live}
		if s.Alerts != nil {
			s.Asks.Alerts = s.Alerts
		}
	}
	if s.Health == nil {
		s.Health = &health.Engine{Store: s.Store, Version: s.Version}
	}
	if s.Updates == nil {
		s.Updates = &updates.Checker{Store: s.Store, Version: s.Version}
	}
	if s.Health.UpdateCheck == nil {
		WireUpdateChecks(s.Health, s.Updates)
	}
	if s.Media == nil && s.Secrets != nil {
		s.Media = &mediaservers.Linker{Store: s.Store, Vault: s.Secrets}
	}
	if s.Subtitles == nil {
		s.Subtitles = subtitles.NewService(s.Store, s.Secrets, s.Live, s.Queue)
	}
	if s.Search == nil {
		q := s.Queue
		var pub sources.Publisher
		if s.Live != nil {
			pub = s.Live // a nil hub must stay a nil Publisher
		}
		s.Search = &sources.Searcher{Store: s.Store, Live: pub, Secrets: s.Secrets, Queue: q,
			OnChosen: func(ctx context.Context, slotID, releaseID int64) {
				if q == nil {
					return
				}
				if _, _, err := q.Enqueue(ctx, downloads.KindGrab, &domain.Subject{Type: "slot", ID: slotID}, map[string]any{"release_id": releaseID},
					"grab:"+itoa(slotID), domain.System); err != nil {
					log.Printf("queue grab: %v", err)
				}
			}}
	}
	a := &API{s: s, media: &mediaservers.Handlers{Store: s.Store, Vault: s.Secrets, Linker: s.Media}}
	a.mf = &mediafiles.Handlers{Deps: mediafiles.Deps{Store: s.Store, Queue: s.Queue, Metadata: s.Metadata, KidMax: a.kidMax, RouteUsable: routeUsable}}
	a.mf.DetailsRating = a.detailsRating
	a.mf.KindShown = a.kindShown
	a.mf.TitleBatch = titleview.WithLookupMemo
	a.mf.AlbumNames = a.albumNames
	a.mf.TitleView = func(ctx context.Context, t mediafiles.Title) (any, error) { return a.titleView(ctx, t) }
	a.mf.CardArt = a.libraryCardArt
	a.watchOn.Metadata = s.Metadata
	a.signin.Store, a.signin.Metadata, a.signin.ServeArtwork = s.Store, s.Metadata, serveArtwork
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
	a.disc = a.newDiscover()
	a.titles = a.newTitlePages()
	a.trailers = a.newTrailers()
	a.ai = &ai.Handlers{Store: s.Store, Version: s.Version, PartOn: a.partIsOn}
	a.shows = shows.NewHandlers(shows.Deps{Store: s.Store, Library: s.Library, Metadata: s.Metadata, AnimeMap: s.AnimeMap,
		KidMax: a.kidMax, KidLimited: a.kidLimited, KidShows: a.kidShows, KindsOn: a.kindsOn, LibraryRating: a.libraryRating,
		Caching: a.exploreCaching, CachedDetails: a.loadedDetails, Details: a.cachedDetails,
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
				a.publishTitleView(v)
			}
			return v, nil
		},
		KindDefaultsFor: a.kindDefaultsFor, TitleID: a.titleID, StateLabels: StateLabels, Trim: trimEpisodeSlots, LibraryError: libraryError})
	// newTrailers built the music handlers before the trailers and shows existed.
	mh := a.mh()
	mh.Trailers, mh.SlotOfTitle, mh.SeasonLabel = a.trailers, a.shows.SlotOfTitle, a.shows.SeasonLabel
	a.dl = &downloads.Web{Store: s.Store, DLSync: s.DLSync, Downloads: s.Downloads, Live: s.Live, Queue: s.Queue,
		Secrets: s.Secrets, Auth: s.Auth, Library: s.Library, Metadata: s.Metadata, Search: s.Search,
		ShowSource: a.shows.ShowSource, AddMovieTitle: a.addMatchedMovie, AddShowTitle: a.addMatchedShow, LibraryError: libraryError}
	a.controls = &setupmod.Handlers{Deps: setupmod.Deps{Store: s.Store, Parts: s.Parts, SafeMode: s.SafeMode, States: s.States, ExitApp: s.ExitApp,
		TitleAllowed: a.titleAllowed, TestDownloader: a.dl.TestDownloader,
		TestSource: func(w http.ResponseWriter, r *http.Request) { a.sourceHandlers().TestSource(w, r) },
		TestSubtitleSite: func(w http.ResponseWriter, r *http.Request) {
			subtitles.NewHandlers(a.s.Store, a.s.Secrets, a.s.Subtitles).ServeSiteTest(w, r)
		}}}
	if s.Media != nil {
		a.controls.TestMediaServer = a.media.Test
	}
	a.dlh = &downloads.Handlers{Deps: downloads.Deps{Store: s.Store, Library: s.Library, Queue: s.Queue,
		TitleID: a.titleID, PublishTitle: a.publishTitle, LibraryError: libraryError,
		VersionJSON: func(ctx context.Context, v mediafiles.Version) any {
			return titleview.VersionViewOf(ctx, s.Store.Read(), v)
		}}}
	return a
}

// partIsOn reports whether the part p is on (always, without a Control).
// In safe mode the Library still lists every title (parts.SafeLibrary).
func (a *API) partIsOn(p parts.Part) bool { return parts.PartOn(a.s.Parts, p) }

// viewer is the signed-in person's id (1, the owner, without sign-in).
func viewer(ctx context.Context) int64 {
	if who, ok := webserver.From(ctx); ok {
		return who.UserID
	}
	return 1
}

// byHandRoutes registers the import-by-hand routes (internal/importer).
func (a *API) byHandRoutes(m *http.ServeMux) {
	h := &importer.Handlers{Deps: importer.Deps{Store: a.s.Store, SavePath: a.s.SavePath}}
	if h.SavePath == "" && a.s.Library != nil {
		h.SavePath = a.s.Library.SavePath
	}
	if a.s.Live != nil {
		h.Live = a.s.Live // a nil hub must stay a nil Publisher
	}
	h.Routes(m)
}

// WireUpdateChecks hands the update check to the health checks: health
// shows its row and its state, and never imports updates.
func WireUpdateChecks(he *health.Engine, uc *updates.Checker) {
	he.UpdateState = func(ctx context.Context) health.UpdateState {
		i := uc.Update(ctx)
		return health.UpdateState{On: i.On, Feed: i.Feed, CheckedAt: i.CheckedAt, Every: uc.Every(ctx), Latest: i.Latest, Newer: i.Newer}
	}
	he.UpdateCheck = func(ctx context.Context) (string, bool, bool) {
		i, ran := uc.Scheduled(ctx)
		return i.Latest, i.Newer, ran
	}
}

// actor is the signed-in person; without sign-in (tests) it is the owner.
func actor(r *http.Request) domain.Actor {
	return webserver.ActorOr(r, domain.Actor{Kind: "user", ID: 1})
}

// Routes registers every route on rt. Modules that have left this package
// register themselves from cmd/holyarrs/wiring.go instead.
func (a *API) Routes(rt *webserver.Router) {
	m := rt.Mux()
	m.HandleFunc("GET /api/v1/titles", a.listTitles)
	if a.s.Store != nil { // some tests build the API without a store
		(&downloads.TagHandlers{DB: a.s.Store.Read()}).Routes(m)
	}
	a.ownedHandlers().Routes(m)
	m.HandleFunc("GET /api/v1/search/suggest", a.searchSuggest)
	m.HandleFunc("GET /api/v1/search", a.plainSearch)
	m.HandleFunc("GET /api/v1/home", a.home)
	m.HandleFunc("POST /api/v1/titles", a.addTitle)
	m.HandleFunc("GET /api/v1/titles/lookup", a.lookupTitles)
	m.HandleFunc("GET /api/v1/titles/{id}", a.getTitle)
	m.HandleFunc("PUT /api/v1/titles/{id}", a.editTitle)
	m.HandleFunc("POST /api/v1/titles/{id}/move", a.moveTitle)
	(&library.VersionHandlers{Store: a.s.Store, Lib: a.s.Library, TitleID: a.titleID, Publish: a.publishTitle, View: func(ctx context.Context, db database.Querier, v mediafiles.Version) any {
		return titleview.VersionViewOf(ctx, db, v)
	}, Fail: libraryError, DownloadPaths: a.downloadPaths}).Routes(m)
	m.HandleFunc("POST /api/v1/titles/{id}/refresh", a.refreshTitle)
	m.HandleFunc("POST /api/v1/titles/refresh", a.refreshTitles)
	m.HandleFunc("GET /api/v1/calendar", a.calendar)
	m.HandleFunc("GET /api/v1/artwork/{name}", a.artwork)
	m.HandleFunc("GET /api/v1/artwork/tmdb/{size}/{file}", a.tmdbArtwork)
	m.HandleFunc("GET /api/v1/artwork/img/{kind}/{id}/{file}", a.serverArtwork)
	a.statusHandlers().Routes(m)
	(&requests.TitleDefaultsHandlers{Store: a.s.Store, Actor: actor}).Routes(m)
	a.dlh.Routes(m)
	m.HandleFunc("GET /api/v1/library-folders", a.listFolders)
	m.HandleFunc("POST /api/v1/library-folders", a.addFolder)
	m.HandleFunc("PUT /api/v1/library-folders/{id}", a.editFolder)
	m.HandleFunc("DELETE /api/v1/library-folders/{id}", a.deleteFolder)
	m.HandleFunc("POST /api/v1/library-folders/{id}/check", a.checkFolder)
	m.HandleFunc("GET /api/v1/never-add", a.listNeverAdd)
	m.HandleFunc("POST /api/v1/never-add", a.addNeverAdd)
	m.HandleFunc("DELETE /api/v1/never-add/{id}", a.deleteNeverAdd)
	m.HandleFunc("GET /api/v1/settings/release-gate", a.getGateDefaults)
	m.HandleFunc("PUT /api/v1/settings/release-gate", a.putGateDefaults)
	lists := &requests.OutsideListHandlers{Store: a.s.Store, Queue: a.s.Queue, Secrets: a.s.Secrets, Actor: actor, LibraryError: libraryError}
	lists.Routes(m)
	(&sources.TitleSearchHandlers{Store: a.s.Store, Queue: a.s.Queue, Show: (&shows.SearchHandlers{Store: a.s.Store, Queue: a.s.Queue}).Search, Artist: a.mh().SearchArtist}).Routes(m)
	m.HandleFunc("GET /api/v1/downloads", a.listDownloads)
	m.HandleFunc("GET /api/v1/imports", a.listImports)
	m.HandleFunc("GET /api/v1/events", a.listEvents)
	m.HandleFunc("GET /api/v1/jobs", a.listJobs)
	a.disc.PeopleRoutes(m)
	a.dlh.CopyRoutes(m)
	(&movies.Handlers{Lib: a.s.Library, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs}).Routes(m)
	a.mf.Routes(m)
	a.dl.Routes(m)
	(&library.OrganiseHandlers{OrganiseDeps: library.OrganiseDeps{Store: a.s.Store, Lib: a.s.Library, Queue: a.s.Queue, TitleID: a.titleID, Fail: libraryError}}).Routes(m)
	a.byHandRoutes(m)
	a.holding().Routes(m)
	a.healthHandlers().Routes(m)
	a.media.Routes(m)
	a.shows.Routes(m)
	a.mh().RouteMux(m)
	subtitles.NewHandlers(a.s.Store, a.s.Secrets, a.s.Subtitles).Routes(m)
	a.appHandlers().Routes(m)
	(&updates.Handlers{Checker: a.s.Updates}).Routes(m)
	(&datapack.Handlers{Deps: datapack.Deps{Store: a.s.Store}}).Routes(m)
	a.scheduleHandlers().Routes(m)
	m.HandleFunc("GET /api/v1/metadata/search", a.searchMetadata)
	(&metadata.Handlers{Up: func(ctx context.Context) bool { return a.s.MetadataUp == nil || a.s.MetadataUp(ctx) }}).Routes(m)
	a.disc.Routes(m)
	a.shows.AnimeRoutes(m)
	a.trailers.Routes(m)
	a.uiHandlers().Routes(m)
	a.signin.Routes(m)
	m.HandleFunc("GET /api/v1/explore/head", a.exploreHead)
	m.HandleFunc("GET /api/v1/explore/tail", a.exploreTail)
	m.HandleFunc("GET /api/v1/explore/top", a.exploreTop)
	m.HandleFunc("GET /api/v1/explore/genre", a.exploreGenre)
	m.HandleFunc("GET /api/v1/explore/genre-row", a.exploreGenreRow)
	m.HandleFunc("GET /api/v1/explore/new", a.exploreNew)
	m.HandleFunc("GET /api/v1/explore/new/services", a.exploreNewServices)
	m.HandleFunc("GET /api/v1/explore/new/service", a.exploreNewService)
	m.HandleFunc("GET /api/v1/explore/browse", a.searchBrowse)
	m.HandleFunc("GET /api/v1/explore/browse/section", a.searchBrowseSection)
	m.HandleFunc("GET /api/v1/explore/browse/category", a.searchBrowseCategory)
	m.HandleFunc("POST /api/v1/explore/counts", a.exploreCounts)
	m.HandleFunc("GET /api/v1/explore/strip", a.exploreStrip)
	m.HandleFunc("POST /api/v1/explore/strip/counts", a.exploreStripCounts)
	m.HandleFunc("DELETE /api/v1/explore/strip/counts", a.exploreStripReset)
	if startup.FeatureOn(features.RowTitleStats) {
		m.HandleFunc("GET /api/v1/explore/title-stats", a.exploreTitleStats)
	}
	a.titles.Routes(m)
	if a.s.Secrets != nil {
		if startup.FeatureOn(features.TraktLists) {
			lists.TraktKeyRoutes(m)
			a.disc.TokenRoutes(m)
		}
		(&shows.TVDBKeyHandlers{Secrets: a.s.Secrets}).Routes(m)
	}
	a.askHandlers().Routes(m)
	(&webserver.URLBaseHandlers{Store: a.s.Store, InUse: a.s.URLBase}).Routes(m)
	(&alerts.Handlers{Store: a.s.Store, Secrets: a.s.Secrets, Sender: a.s.Alerts, Caller: func(r *http.Request) (int64, string) { p := a.person(r); return p.UserID, p.Name },
		OwnEmail: func(ctx context.Context, userID int64) (string, error) {
			t, err := requests.ReadTargets(ctx, a.s.Store.Read(), userID)
			return t.Email, err
		}}).Routes(m)
	if startup.FeatureOn(features.PlexWatchlists) {
		(&requests.WatchlistHandlers{Store: a.s.Store, Queue: a.s.Queue, Watchlists: func() *requests.Watchlists { return a.s.Watchlists }, Person: a.person}).Routes(m)
	}
	(&requests.ReportHandlers{Svc: a.s.Asks, Store: a.s.Store, Person: a.person, KidMax: a.kidMax, TitleGate: a.controls.ReportTitleGate}).Routes(m)
	(&requests.AskedByHandlers{Store: a.s.Store, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs}).Routes(m)
	(&requests.FollowHandlers{Svc: a.s.Asks, Person: a.person, Title: a.titleID, Changed: a.mh().FollowsChanged}).Routes(m)
	a.controls.Routes(m)
	m.HandleFunc("GET /api/v1/openapi.json", a.openapi)
	m.HandleFunc("GET /api/v1/health", a.health)
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	// readyz answers 200 once the app has fully started (until then the
	// start page answers it with 503).
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ready\n")) })
	a.liveRoutes(m)
	m.Handle("GET /", webserver.Handler())
	if a.s.Backups != nil {
		a.s.Backups.Routes(m, a.s.Restart)
	}
	if a.s.Auth != nil {
		a.s.Auth.Routes(m)
	}
}

// Handler returns every route behind the sign-in, permission and gate layers.
func (a *API) Handler() http.Handler {
	rt := webserver.NewRouter()
	a.Routes(rt)
	a.ModuleRoutes(rt)
	return a.Chain(rt)
}

// Chain wraps a router that Routes filled with the layers every request
// passes: sign-in, the permission gate, kid limits, part switches and the
// security headers.
func (a *API) Chain(rt *webserver.Router) http.Handler {
	m := rt.Mux()
	if a.s.Auth != nil {
		a.ai.Inner = a.kidGate(m, a.controls.PartGate(m, accounts.Gate(m)))
		return webserver.SecureHeaders(a.arr().Gate(a.s.Auth.Middleware(wrapMux(a.liveRecheck(a.settingsLog().Wrap(a.status.ForgetOnWrite(a.kidGate(m, a.controls.PartGate(m, accounts.Gate(m))))))))))
	}
	a.ai.Inner = a.controls.PartGate(m, m)
	return webserver.SecureHeaders(a.arr().Gate(wrapMux(a.settingsLog().Wrap(a.status.ForgetOnWrite(a.controls.PartGate(m, m))))))
}

// wrapMux lets tests record which routes the mux served.
var wrapMux = func(h http.Handler) http.Handler { return h }

// healthz is open to everyone: it says only whether the database answers.
func (a *API) healthz(w http.ResponseWriter, _ *http.Request) {
	if _, err := a.s.Store.Version(); err != nil {
		http.Error(w, "database not answering\n", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

// SubtitleFileView is one main File with its subtitles.
type SubtitleFileView = subtitles.FileView

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// SlotView, TitleView, VersionView and ArtistInfo are the titleview module's views.
type (
	SlotView    = titleview.SlotView
	TitleView   = titleview.TitleView
	VersionView = titleview.VersionView
	ArtistInfo  = titleview.ArtistInfo
)

// showError answers show refusals; anything else goes to libraryError.
func showError(w http.ResponseWriter, err error) { shows.ShowError(w, err, libraryError) }

// EpisodeSlot and EpisodeView are the shows module's episode views.
type (
	EpisodeSlot = shows.EpisodeSlot
	EpisodeView = shows.EpisodeView
)

// ShowInfo is what a show's page needs besides the shared Title fields.
type ShowInfo = shows.Info

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (a *API) showInfo(ctx context.Context, t mediafiles.Title) (*ShowInfo, error) {
	return shows.InfoOf(ctx, a.s.Store.Read(), t)
}

// StateLabels are the words people see for Slot states.
var StateLabels = mediafiles.StateLabels

// artworkLinks is titleview.ArtworkLinks.
var artworkLinks = titleview.ArtworkLinks

func (a *API) titleView(ctx context.Context, t mediafiles.Title) (TitleView, error) {
	return a.tv().View(ctx, t)
}

// tv is the title view builder over this API's services.
func (a *API) tv() *titleview.Handlers {
	return titleview.New(titleview.Deps{Store: a.s.Store, Asks: a.s.Asks, Live: a.s.Live})
}

func (a *API) listTitles(w http.ResponseWriter, r *http.Request) {
	ts, err := mediafiles.SearchTitles(r.Context(), a.s.Store, r.URL.Query().Get("q"))
	if errors.Is(err, mediafiles.ErrTitleSearchQuery) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if r.URL.Query().Has("brief") {
		a.listTitlesBrief(w, r, ts)
		return
	}
	out := []TitleView{}
	who, signed := webserver.From(r.Context())
	manage := !signed || who.Has(webserver.PermManageLibrary)
	ctx, err := titleview.WithTitleFacts(titleview.WithLookupMemo(r.Context()), a.s.Store.Read())
	if err == nil && len(ts) > 0 {
		ctx, err = a.tv().WithTitleBatch(ctx)
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for _, t := range ts {
		if p, ok := parts.ForTitleKind(t.Kind); ok && !a.partIsOn(p) && !a.s.Parts.SafeMode {
			continue // its part is off: hidden, not removed (safe mode lists every title)
		}
		if !a.titleAllowed(r, t) {
			continue // above a Kid's age-rating limit
		}
		v, err := a.titleView(ctx, t)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if v.Artist != nil {
			v.Artist.Bio = mediafiles.KeptArtistBio(r.Context(), v.Artist.MBID, t.Name, a.mh().SideKept)
		}
		if !manage {
			v.Copy = nil
		}
		out = append(out, v)
	}
	webserver.WriteTaggedJSON(w, r, map[string]any{"titles": out})
}

func (a *API) titleID(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	return a.tv().TitleID(w, r)
}

func limit(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return def
	}
	return n
}

func (a *API) listDownloads(w http.ResponseWriter, r *http.Request) {
	ds, err := database.Select[domain.Download](r.Context(), a.s.Store.Read(), "downloads", "1=1 ORDER BY id DESC LIMIT ?", limit(r, 50))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if ds == nil {
		ds = []domain.Download{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"downloads": ds})
}

func (a *API) listImports(w http.ResponseWriter, r *http.Request) {
	is, err := database.Select[mediafiles.Import](r.Context(), a.s.Store.Read(), "imports", "1=1 ORDER BY id DESC LIMIT ?", limit(r, 20))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if is == nil {
		is = []mediafiles.Import{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"imports": is})
}

// EventView is one History row: an Event with its movie's name.
type EventView struct {
	domain.Event
	TitleName string `json:"title_name,omitempty"`
	// Episode names the episodes of a show Event: S01E02, or S01E01-E03
	// for a pack.
	Episode string `json:"episode,omitempty"`
	// Version names the Version (such as 4K) the Event is about, when its
	// title has more than one.
	Version string `json:"version,omitempty"`
	// ActorName is the member who did it, so History can say who without
	// the admin-only member list.
	ActorName string `json:"actor_name,omitempty"`
}

// parseDay reads a date (2006-01-02) or a full RFC 3339 time.
func parseDay(v string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, v)
}

// listEvents is History: ?title=<id>&kind=a,b,import_*&from=&to=&limit=&offset=.
// to is a day that is included (or an exact time that is not).
func (a *API) listEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := database.EventFilter{Limit: limit(r, 50)}
	if v := q.Get("title"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "title must be a movie or show id")
			return
		}
		f.TitleID = id
	}
	if v := q.Get("episode"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "episode must be an episode id")
			return
		}
		var titleID int64
		if err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT title_id FROM items WHERE id = ? AND kind = 'episode'", id).Scan(&titleID); err != nil {
			webserver.Fail(w, http.StatusBadRequest, "no such episode")
			return
		}
		f.TitleID, f.ItemIDs, f.SlotIDs = titleID, []int64{id}, []int64{}
		rows, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT id FROM slots WHERE item_id = ?", id)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for rows.Next() {
			var sid int64
			if err := rows.Scan(&sid); err != nil {
				rows.Close()
				webserver.Internal(w, "api", err)
				return
			}
			f.SlotIDs = append(f.SlotIDs, sid)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	// season=<n> with title=<id>: the history of one season's episodes.
	if v := q.Get("season"); v != "" && f.TitleID > 0 {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			webserver.Fail(w, http.StatusBadRequest, "season must be a number")
			return
		}
		rows, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT i.id, s.id FROM items i LEFT JOIN slots s ON s.item_id = i.id WHERE i.title_id = ? AND i.kind = 'episode' AND json_extract(i.numbers, '$.season') = ?", f.TitleID, n)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		f.ItemIDs, f.SlotIDs = []int64{}, []int64{}
		seen := map[int64]bool{}
		for rows.Next() {
			var iid int64
			var sid *int64
			if err := rows.Scan(&iid, &sid); err != nil {
				rows.Close()
				webserver.Internal(w, "api", err)
				return
			}
			if !seen[iid] {
				seen[iid] = true
				f.ItemIDs = append(f.ItemIDs, iid)
			}
			if sid != nil {
				f.SlotIDs = append(f.SlotIDs, *sid)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if len(f.ItemIDs) == 0 {
			f.ItemIDs = []int64{-1}
		}
	}
	if v := q.Get("album"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "album must be an album id")
			return
		}
		var titleID int64
		if err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT title_id FROM parts WHERE id = ? AND kind = 'album'", id).Scan(&titleID); err != nil {
			webserver.Fail(w, http.StatusBadRequest, "no such album")
			return
		}
		f.TitleID, f.PartID, f.ItemIDs, f.SlotIDs = titleID, id, []int64{}, []int64{}
		rows, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT i.id, s.id FROM items i LEFT JOIN slots s ON s.item_id = i.id WHERE i.part_id = ? AND i.kind = 'track'", id)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for rows.Next() {
			var iid int64
			var sid sql.NullInt64
			if err := rows.Scan(&iid, &sid); err != nil {
				rows.Close()
				webserver.Internal(w, "api", err)
				return
			}
			f.ItemIDs = append(f.ItemIDs, iid)
			if sid.Valid {
				f.SlotIDs = append(f.SlotIDs, sid.Int64)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	for _, k := range strings.Split(q.Get("kind"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			// History's default list (every kind but routine) is about 52 long.
			if len(k) > 60 || len(f.Kinds) >= 64 {
				webserver.Fail(w, http.StatusBadRequest, "kind is a comma list of at most 64 Event kinds")
				return
			}
			f.Kinds = append(f.Kinds, k)
		}
	}
	// source=<id>: one Source's grabs and pauses (owners and admins see the Source list).
	if v := q.Get("source"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "source must be a Source id")
			return
		}
		f.SourceID = id
	}
	// Feed checks are routine: History's today look hides them with feed=0,
	// unless the kinds ask for them.
	f.HideFeed = q.Get("feed") == "0"
	for _, k := range f.Kinds {
		if k == "feed_checked" || k == "feed_*" {
			f.HideFeed = false
		}
	}
	if v := q.Get("from"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "from must be a date like 2026-10-01")
			return
		}
		f.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "to must be a date like 2026-10-31")
			return
		}
		if len(v) == len("2006-01-02") {
			t = t.AddDate(0, 0, 1)
		}
		f.To = t
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			webserver.Fail(w, http.StatusBadRequest, "offset must be 0 or more")
			return
		}
		f.Offset = n
	}
	// The audit trail (sign-ins, API keys, settings changes) is only for
	// the owner and people who manage users or settings.
	if who, signed := webserver.From(r.Context()); signed && !who.MaySeeAudit() {
		f.HideKinds = database.AuditEventKinds
	}
	// Asks' Events name who asked and who decided, with the decline note:
	// only for people who approve, as on the asks routes.
	if who, signed := webserver.From(r.Context()); signed && !who.Has(webserver.PermApprove) {
		f.HideKinds = append(slices.Clone(f.HideKinds), domain.AskEventKinds...)
	}
	if f.SourceID > 0 {
		ids, err := sources.ReleaseIDs(r.Context(), a.s.Store.Read(), f.SourceID)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		f.SourceReleaseIDs = ids
	}
	es, total, err := a.s.Store.Events(r.Context(), f)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// The filtered episode, named for the screen, is read with the page.
	var filtered *domain.Event
	if len(f.ItemIDs) == 1 {
		filtered = &domain.Event{TitleID: &f.TitleID, Data: map[string]any{"item_id": float64(f.ItemIDs[0])}}
	}
	page, err := shows.LoadEventPage(r.Context(), a.s.Store.Read(), shows.AppendEvent(es, filtered))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// Member names for the rows a person did (a missing name stays empty).
	var people []int64
	for _, e := range es {
		// An AI app acts with a member's key; its rows name that member too.
		if (e.Actor.Kind == "user" || e.Actor.Kind == "ai") && e.Actor.ID != 0 && !slices.Contains(people, e.Actor.ID) {
			people = append(people, e.Actor.ID)
		}
	}
	names := map[int64]string{}
	_ = eachIn(r.Context(), a.s.Store.Read(), `SELECT id, name FROM users WHERE id IN (%s)`, people, func(rs *sql.Rows) error {
		var id int64
		var n string
		if err := rs.Scan(&id, &n); err != nil {
			return err
		}
		names[id] = n
		return nil
	})
	out := make([]EventView, 0, len(es))
	for _, e := range es {
		v := EventView{Event: e}
		if e.Actor.Kind == "user" || e.Actor.Kind == "ai" {
			v.ActorName = names[e.Actor.ID]
		}
		if e.TitleID != nil {
			v.TitleName = page.TitleName(e)
			v.Episode = page.Episode(e)
			v.Version = page.Version(e)
		}
		out = append(out, v)
	}
	resp := map[string]any{"events": out, "total": total, "limit": f.Limit, "offset": f.Offset}
	if filtered != nil {
		resp["episode"] = page.Episode(*filtered)
	}
	webserver.WriteJSON(w, http.StatusOK, resp)
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	js, err := database.Select[domain.Job](r.Context(), a.s.Store.Read(), "jobs", "1=1 ORDER BY id DESC LIMIT ?", limit(r, 50))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if js == nil {
		js = []domain.Job{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"jobs": js})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, a.healthView(r.Context()))
}

// healthView is what GET /api/v1/health answers.
func (a *API) healthView(ctx context.Context) map[string]any {
	db := a.s.Store.Read()
	ver, err := a.s.Store.Version()
	ok := err == nil
	type part struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	var srcs, dls []part
	rows, err := db.QueryContext(ctx, "SELECT name, state FROM sources ORDER BY id")
	if err == nil {
		for rows.Next() {
			var p part
			_ = rows.Scan(&p.Name, &p.State)
			srcs = append(srcs, p)
		}
		rows.Close()
	}
	rows, err = db.QueryContext(ctx, "SELECT name, state FROM downloaders ORDER BY id")
	if err == nil {
		for rows.Next() {
			var p part
			_ = rows.Scan(&p.Name, &p.State)
			dls = append(dls, p)
		}
		rows.Close()
	}
	p95, n := a.s.Store.WriteWaitP95()
	clients := 0
	if a.s.Live != nil {
		clients = a.s.Live.Clients()
	}
	return map[string]any{
		"ok": ok, "version": a.s.Version, "schema": ver, "uptime_s": int(time.Since(a.s.Started).Seconds()),
		"write_wait_p95_ms": float64(p95.Microseconds()) / 1000, "write_wait_samples": n, "writes": a.s.Store.WriteCount(),
		"sources": srcs, "downloaders": dls, "live_clients": clients, "url_base": a.s.URLBase,
		"music_covers": a.mh().PreloadStatus(),
	}
}

func (a *API) searchMetadata(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" || len(q) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "send ?q=<name>, and &year=<year> if you know it")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	ms, err := a.s.Metadata.Search(r.Context(), q, year)
	if metadataServiceError(w, err) {
		return
	}
	if errors.Is(err, metadata.ErrNoAccess) {
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if ms == nil {
		ms = []metadata.Movie{}
	}
	ms = a.kidMovies(r.Context(), a.kidMax(r), ms)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"movies": ms, "attribution": metadata.Attribution})
}

// StartSpeedHistory starts the total-speed readings (downloads.SpeedHistory).
func (a *API) StartSpeedHistory(ctx context.Context) {
	if a.s.Store != nil {
		a.dl.Speeds.Start(ctx, a.s.Store.Read())
	}
}

// healthHandlers build the health module's routes; downloads, the parts
// and the update check reach it as funcs.
func (a *API) healthHandlers() *health.Handlers {
	h := &health.Handlers{Store: a.s.Store, Engine: a.s.Health, States: a.s.States, SafeMode: a.s.SafeMode,
		SyncTrouble: func(ctx context.Context) any { return a.dl.SyncTrouble(ctx) },
		DefaultFeed: updates.DefaultReleaseFeed, ResetUpdate: a.s.Updates.ResetUpdate}
	if a.s.Parts != nil {
		h.PartStatuses = func() []jobs.Status { return a.s.Parts.Statuses() }
	}
	return h
}

// statusHandlers serve the Settings status (health); backups, the update
// check and sign-in reach it as funcs.
func (a *API) statusHandlers() *health.StatusHandlers {
	h := &health.StatusHandlers{Store: a.s.Store, Engine: a.s.Health, States: a.s.States,
		MetadataUp: func(ctx context.Context) (bool, bool) {
			if a.s.MetadataUp == nil {
				return false, false
			}
			return a.s.MetadataUp(ctx), true
		},
		UpdateNewer: func(ctx context.Context) bool { return a.s.Updates.Update(ctx).Newer }, Cache: &a.status}
	if a.s.Parts != nil {
		h.PartStatuses = func() []jobs.Status { return a.s.Parts.Statuses() }
	}
	if a.s.Backups != nil {
		h.NewestBackup = func() (string, bool) {
			bs, err := a.s.Backups.List()
			if err != nil {
				return "", false
			}
			newest := ""
			var at time.Time
			for _, b := range bs {
				if b.CreatedAt.After(at) {
					at = b.CreatedAt
					newest = b.CreatedAt.UTC().Format(time.RFC3339)
				}
			}
			return newest, true
		}
	}
	if a.s.Auth != nil {
		h.SkipLocal, h.OutsideJoin = a.s.Auth.SkipLocal, a.s.Auth.OutsideJoinOpen
	}
	return h
}

// settingsLog writes the settings_changed Events (health).
func (a *API) settingsLog() *health.SettingsLog {
	return &health.SettingsLog{Store: a.s.Store, Who: whoName, Actor: actor}
}

// scheduleHandlers build the health module's Schedules routes.
func (a *API) scheduleHandlers() *health.ScheduleHandlers {
	return &health.ScheduleHandlers{Store: a.s.Store, Queue: a.s.Queue, States: a.s.States, Parts: a.s.Parts, SafeMode: a.s.SafeMode}
}

// appHandlers build the storage, About, log and support bundle routes (health).
func (a *API) appHandlers() *health.AppHandlers {
	return &health.AppHandlers{Store: a.s.Store, Version: a.s.Version, Started: a.s.Started, Log: a.s.Log, Secrets: a.s.Secrets,
		ConfigDir: a.s.ConfigDir, SavePath: a.s.SavePath, HealthView: func(ctx context.Context) any { return a.healthView(ctx) }}
}

// uiHandlers build the UI settings routes (health).
func (a *API) uiHandlers() *health.UIHandlers {
	h := &health.UIHandlers{Store: a.s.Store, Actor: actor, CountryCount: a.s.CountryCount}
	if a.s.Library != nil && a.s.Library.Artwork != nil {
		h.SetArtworkCap = a.s.Library.Artwork.SetCap
	}
	return h
}

// ApplyCountryCount passes the stored "Count this install" on to the
// requests to the metadata server (at start and after each change).
func (a *API) ApplyCountryCount(ctx context.Context) { a.uiHandlers().ApplyCountryCount(ctx) }

// idChunk keeps an IN list under SQLite's variable limit.
const idChunk = 500

// eachIn runs query (with one "%s" for the IN list) over ids in chunks.
func eachIn(ctx context.Context, db *sql.DB, query string, ids []int64, scan func(*sql.Rows) error) error {
	for start := 0; start < len(ids); start += idChunk {
		part := ids[start:min(start+idChunk, len(ids))]
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf(query, strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")), args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			if err := scan(rows); err != nil {
				_ = rows.Close()
				return err
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}
