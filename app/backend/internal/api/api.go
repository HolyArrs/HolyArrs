// Package api serves the versioned HTTP API under /api/v1, the live stream
// at /live and the web app at /.
package api

import (
	"context"
	"database/sql"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/movies"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"

	"github.com/HolyArrs/holyarrs/app/backend/internal/ai"

	"log"
	"net/http"
	"net/url"
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
	dl *downloads.Web
	// interest limits each session's live interest calls (webserver.go).
	interest interestState
	// listSeen is each region's last world-list shelves signature
	// (music_lists_v1), guarded by musicv2.cursorMu.
	listSeen map[string]string
	// lookups counts each person's music lookups in flight.
	lookups inFlight
	// confirm holds the two-step controls' confirmation ids.
	confirm webserver.Confirms
	// ai serves the AI connections (internal/ai).
	ai *ai.Handlers
	// discover caches Explore rows and details pages and limits each person.
	discover discoverState
	// rated keeps TMDB ratings looked up for Kid limits.
	rated ratingState
	// status keeps the Settings status answer for 30 seconds.
	status health.StatusCache
	// serviceTop keeps the per-service top lists for 12 hours.
	serviceTop serviceTopCache
	// watchOn keeps each title's TMDB watch providers per region a day.
	watchOn watchOnCache
	// music keeps the music trending lists for an hour.
	music musicTrendCache
	// side keeps music page side data, also in lookup_cache (musicside.go).
	side   sideStore
	rising risingCache
	// pics are the metadata server's music picture records (musicpictures.go).
	pics picRecords
	// musicx keeps artist genres and similar artists for Explore → Music.
	musicx musicExplore
	// musicAns keeps each member's Explore → Music answers a minute.
	musicAns musicAnswers
	// musicv2 holds the music_rows_v2 cache layers.
	musicv2 musicV2State
	// genreArt keeps genre tile pictures.
	genreArt music.GenreArtCache
	// signin keeps the sign-in page's art and its per-address budget.
	signin signinArtState
	// signinSeed keeps the first sign-in slide's tiny thumbnails.
	signinSeed signinSeedState
	// artMisses is each person's budget of artwork cache misses.
	artMisses discover.ArtworkBudget
	// explore runs the Explore pool job and keeps its lists and pages.
	explore   exploreState
	genreRows genreRowCache
	// trailers keeps resolved trailer addresses, serves the trailer routes
	// and runs the trailer pass (metadata).
	trailers *metadata.Trailers
	// artistAsked notes artists whose music videos the pass keeps ready.
	artistAsked artistAsked
	// warm fills listed titles' details, extras and trailers ahead.
	warm titleWarmer
	// ahead runs music fetch-ahead; ctl keeps the server's switch.
	ahead   musicAhead
	artWarm artWarmer
	// musicPre stores every engine page's covers ahead (musicpreload.go).
	musicPre musicPreload
	ctl      musicControl
	// artistVideos keeps each artist's music videos (see music_video.go).
	artistVideos artistVideoCache
}

// New makes the API.
func New(s Server) *API {
	accounts.SetViewKeyCheck(exploreViewKeyOK)
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
	a.titles = a.newTitlePages()
	a.trailers = a.newTrailers()
	a.ai = &ai.Handlers{Store: s.Store, Version: s.Version, PartOn: a.partIsOn}
	a.dl = &downloads.Web{Store: s.Store, DLSync: s.DLSync, Downloads: s.Downloads, Live: s.Live, Queue: s.Queue,
		Secrets: s.Secrets, Auth: s.Auth, Library: s.Library, Metadata: s.Metadata, Search: s.Search,
		ShowSource: a.showSource, AddMovieTitle: a.addMatchedMovie, AddShowTitle: a.addMatchedShow, LibraryError: libraryError}
	return a
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
	m.HandleFunc("GET /api/v1/library/search", a.librarySearch)
	m.HandleFunc("GET /api/v1/search/suggest", a.searchSuggest)
	m.HandleFunc("GET /api/v1/search", a.plainSearch)
	m.HandleFunc("GET /api/v1/library-summary", a.librarySummary)
	m.HandleFunc("GET /api/v1/home", a.home)
	m.HandleFunc("POST /api/v1/titles", a.addTitle)
	m.HandleFunc("GET /api/v1/titles/lookup", a.lookupTitles)
	m.HandleFunc("GET /api/v1/titles/{id}", a.getTitle)
	m.HandleFunc("PUT /api/v1/titles/{id}", a.editTitle)
	m.HandleFunc("POST /api/v1/titles/{id}/move", a.moveTitle)
	(&library.VersionHandlers{Store: a.s.Store, Lib: a.s.Library, TitleID: a.titleID, Publish: a.publishTitle, View: func(ctx context.Context, db database.Querier, v mediafiles.Version) any {
		return versionView(ctx, db, v)
	}, Fail: libraryError, DownloadPaths: a.downloadPaths}).Routes(m)
	m.HandleFunc("POST /api/v1/titles/{id}/refresh", a.refreshTitle)
	m.HandleFunc("POST /api/v1/titles/refresh", a.refreshTitles)
	m.HandleFunc("GET /api/v1/calendar", a.calendar)
	m.HandleFunc("GET /api/v1/artwork/{name}", a.artwork)
	m.HandleFunc("GET /api/v1/artwork/tmdb/{size}/{file}", a.tmdbArtwork)
	m.HandleFunc("GET /api/v1/artwork/img/{kind}/{id}/{file}", a.serverArtwork)
	a.statusHandlers().Routes(m)
	m.HandleFunc("GET /api/v1/settings/title-defaults", a.getTitleDefaults)
	m.HandleFunc("PUT /api/v1/settings/title-defaults", a.putTitleDefaults)
	m.HandleFunc("GET /api/v1/pick-rules", a.listPickRules)
	m.HandleFunc("POST /api/v1/pick-rules", a.addPickRules)
	m.HandleFunc("GET /api/v1/pick-rules/{id}", a.getPickRules)
	m.HandleFunc("PUT /api/v1/pick-rules/{id}", a.editPickRules)
	m.HandleFunc("DELETE /api/v1/pick-rules/{id}", a.deletePickRules)
	m.HandleFunc("GET /api/v1/own-traits", a.listOwnTraits)
	m.HandleFunc("POST /api/v1/own-traits", a.addOwnTrait)
	m.HandleFunc("PUT /api/v1/own-traits/{id}", a.editOwnTrait)
	m.HandleFunc("DELETE /api/v1/own-traits/{id}", a.deleteOwnTrait)
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
	(&sources.TitleSearchHandlers{Store: a.s.Store, Queue: a.s.Queue, Show: (&shows.SearchHandlers{Store: a.s.Store, Queue: a.s.Queue}).Search, Artist: a.searchArtist}).Routes(m)
	m.HandleFunc("GET /api/v1/downloads", a.listDownloads)
	m.HandleFunc("GET /api/v1/imports", a.listImports)
	m.HandleFunc("GET /api/v1/events", a.listEvents)
	m.HandleFunc("GET /api/v1/jobs", a.listJobs)
	a.peopleRoutes(m)
	a.copyRoutes(m)
	(&movies.Handlers{Lib: a.s.Library, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs}).Routes(m)
	m.HandleFunc("GET /api/v1/library/copies", a.listLibraryCopies)
	m.HandleFunc("POST /api/v1/library/copies/search/preview", a.previewLibraryCopySearch)
	m.HandleFunc("POST /api/v1/library/copies/search", a.searchLibraryCopies)
	a.dl.Routes(m)
	a.organiseRoutes(m)
	a.manualImportRoutes(m)
	a.holding().Routes(m)
	a.healthRoutes(m)
	a.media.Routes(m)
	a.showRoutes(m)
	a.musicRoutes(m)
	a.libraryHandlers().Routes(m)
	a.musicPageRoutes(m)
	subtitles.NewHandlers(a.s.Store, a.s.Secrets, a.s.Subtitles).Routes(m)
	a.thisAppRoutes(m)
	a.dataPackRoutes(m)
	a.scheduleRoutes(m)
	m.HandleFunc("GET /api/v1/metadata/search", a.searchMetadata)
	(&metadata.Handlers{Up: func(ctx context.Context) bool { return a.s.MetadataUp == nil || a.s.MetadataUp(ctx) }}).Routes(m)
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}", a.metadataDetails)
	m.HandleFunc("GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}", a.metadataSeason)
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}/more-from", a.metadataMoreFrom)
	reviews := a.reviewHandlers()
	reviews.Routes(m)
	a.animeHandlers().Routes(m)
	a.trailers.Routes(m)
	a.uiSettingsRoutes(m)
	a.signinArtRoutes(m)
	m.HandleFunc("GET /api/v1/discover", a.discoverRow)
	m.HandleFunc("GET /api/v1/watch-services", a.watchServices)
	m.HandleFunc("GET /api/v1/explore/head", a.exploreHead)
	m.HandleFunc("GET /api/v1/explore/tail", a.exploreTail)
	m.HandleFunc("GET /api/v1/explore/top", a.exploreTop)
	m.HandleFunc("GET /api/v1/explore/service-top", a.exploreServiceTop)
	m.HandleFunc("GET /api/v1/explore/genre", a.exploreGenre)
	m.HandleFunc("GET /api/v1/explore/genre-row", a.exploreGenreRow)
	m.HandleFunc("GET /api/v1/explore/new", a.exploreNew)
	m.HandleFunc("GET /api/v1/explore/new/services", a.exploreNewServices)
	m.HandleFunc("GET /api/v1/explore/new/service", a.exploreNewService)
	m.HandleFunc("GET /api/v1/explore/new/music", a.exploreNewMusic)
	m.HandleFunc("GET /api/v1/metadata/album/{mbid}/follow", a.getAlbumFollow)
	m.HandleFunc("PUT /api/v1/metadata/album/{mbid}/follow", a.putAlbumFollow)
	m.HandleFunc("DELETE /api/v1/metadata/album/{mbid}/follow", a.deleteAlbumFollow)
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
	m.HandleFunc("GET /api/v1/music-trending", a.musicTrending)
	m.HandleFunc("GET /api/v1/explore/music", a.exploreMusic)
	m.HandleFunc("GET /api/v1/explore/music/rising", a.risingToday)
	m.HandleFunc("GET /api/v1/music-trending/art/{kind}/{mbid}", a.musicTrendArt)
	m.HandleFunc("GET /api/v1/music/genre/{slug}", a.musicGenrePage)
	m.HandleFunc("GET /api/v1/music/browse/{kind}/{slug}", a.musicBrowsePage)
	m.HandleFunc("GET /api/v1/music/browse-tiles", a.musicBrowseTiles)
	a.genreArtHandlers().Routes(m)
	a.titles.Routes(m)
	if a.s.Secrets != nil {
		if startup.FeatureOn(features.TraktLists) {
			lists.TraktKeyRoutes(m)
			reviews.TokenRoutes(m)
		}
		(&shows.TVDBKeyHandlers{Secrets: a.s.Secrets}).Routes(m)
	}
	a.askHandlers().Routes(m)
	(&webserver.URLBaseHandlers{Store: a.s.Store, InUse: a.s.URLBase}).Routes(m)
	(&alerts.Handlers{Store: a.s.Store, Secrets: a.s.Secrets, Sender: a.s.Alerts, Caller: a.callerID, OwnEmail: a.ownEmail}).Routes(m)
	if startup.FeatureOn(features.PlexWatchlists) {
		(&requests.WatchlistHandlers{Store: a.s.Store, Queue: a.s.Queue, Watchlists: func() *requests.Watchlists { return a.s.Watchlists }, Person: a.person}).Routes(m)
	}
	(&requests.ReportHandlers{Svc: a.s.Asks, Store: a.s.Store, Person: a.person, KidMax: a.kidMax, TitleGate: a.reportTitleGate}).Routes(m)
	(&requests.AskedByHandlers{Store: a.s.Store, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs}).Routes(m)
	(&requests.FollowHandlers{Svc: a.s.Asks, Person: a.person, Title: a.titleID, Changed: a.followsChanged}).Routes(m)
	a.controlRoutes(m)
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
		a.ai.Inner = a.kidGate(m, a.partGate(m, accounts.Gate(m)))
		return webserver.SecureHeaders(a.arr().Gate(a.s.Auth.Middleware(wrapMux(a.liveRecheck(a.settingsLog().Wrap(a.status.ForgetOnWrite(a.kidGate(m, a.partGate(m, accounts.Gate(m))))))))))
	}
	a.ai.Inner = a.partGate(m, m)
	return webserver.SecureHeaders(a.arr().Gate(wrapMux(a.settingsLog().Wrap(a.status.ForgetOnWrite(a.partGate(m, m))))))
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

// SlotView is a Slot as the page shows it.
type SlotView struct {
	ID         int64            `json:"id"`
	Version    string           `json:"version"`
	VersionID  int64            `json:"version_id"`
	State      string           `json:"state"`
	StateLabel string           `json:"state_label"`
	Reason     string           `json:"reason,omitempty"`
	File       *mediafiles.File `json:"file,omitempty"`
	DownloadID *int64           `json:"active_download_id,omitempty"`
	// Download is the active (or last) Download in plain words.
	Download *downloads.DownloadView `json:"download,omitempty"`
}

// TitleView is a Title with its Slots.
type TitleView struct {
	ID               int64      `json:"id"`
	Kind             string     `json:"kind"`
	Name             string     `json:"name"`
	Year             *int       `json:"year,omitempty"`
	Overview         *string    `json:"overview,omitempty"`
	OriginalLanguage *string    `json:"original_language,omitempty"`
	IDs              any        `json:"ids"`
	KeepUpToDate     bool       `json:"keep_up_to_date"`
	WhyAdded         string     `json:"why_added"`
	ReleaseGate      string     `json:"release_gate"`
	GateDelayDays    int        `json:"gate_delay_days"`
	GateDriver       string     `json:"gate_driver"`
	GateOpensAt      *time.Time `json:"gate_opens_at,omitempty"`
	CinemaDate       *time.Time `json:"cinema_date,omitempty"`
	DigitalDate      *time.Time `json:"digital_date,omitempty"`
	PhysicalDate     *time.Time `json:"physical_date,omitempty"`
	RefreshedAt      *time.Time `json:"refreshed_at,omitempty"`
	DroppedAt        *time.Time `json:"dropped_at,omitempty"`
	Stage            string     `json:"stage"`       // announced, in_cinemas, released
	GateOpen         bool       `json:"gate_open"`   // searching may start
	GateReason       string     `json:"gate_reason"` // why it is or is not searched yet
	// The main Version: its folder and Pick rules.
	LibraryFolderID int64    `json:"library_folder_id,omitempty"`
	FolderPath      string   `json:"folder_path,omitempty"`
	PickRulesID     int64    `json:"pick_rules_id,omitempty"`
	PickRulesName   string   `json:"pick_rules_name,omitempty"`
	Artwork         []string `json:"artwork"`
	// Screen details from the metadata source: wide picture and logo URLs,
	// genre names, runtime, the US rating and people (top cast, directors).
	Backdrop       string   `json:"backdrop,omitempty"`
	Logo           string   `json:"logo,omitempty"`
	Genres         []string `json:"genres"`
	RuntimeMinutes int      `json:"runtime_minutes,omitempty"`
	Rating         string   `json:"rating,omitempty"`
	People         []string `json:"people"`
	// Tags route the title's grabs and searches to the Downloaders and Sources that share one.
	Tags []string `json:"tags"`
	// AddedAt is when the title joined the Library; SizeBytes is the sum of
	// every file kept for it, across all Versions (Library table columns).
	AddedAt   time.Time `json:"added_at"`
	SizeBytes int64     `json:"size_bytes"`
	// Copy is a movie's main file (codec, audio, ids for a copy search); only
	// in the Library list, and only for people who manage the Library.
	Copy *mediafiles.TitleCopy `json:"copy,omitempty"`
	// LastActivity is the newest grab or import for the title, if any.
	LastActivity *time.Time `json:"last_activity,omitempty"`
	// Versions are the wanted copies (main first), such as HD and 4K.
	Versions []VersionView `json:"versions"`
	Slots    []SlotView    `json:"slots"`
	// Asks are the Requests that led to it: who asked and who decided.
	Asks []requests.View `json:"asks"`
	// Show holds a show's choices and season summaries (shows only; their
	// episodes and Slots come from /titles/{id}/episodes, so Slots is empty).
	Show *ShowInfo `json:"show,omitempty"`
	// Artist holds an artist's choices and counts (artists only; albums,
	// Pressings and tracks come from /titles/{id}/albums and /albums/{id}).
	Artist *ArtistInfo `json:"artist,omitempty"`
}

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
var StateLabels = map[mediafiles.SlotState]string{
	mediafiles.SlotNotOutYet: "Not out yet", mediafiles.SlotWanted: "Missing", mediafiles.SlotSearching: "Searching",
	mediafiles.SlotHeld: "Held", mediafiles.SlotChosen: "Chosen", mediafiles.SlotDownloading: "Downloading",
	mediafiles.SlotStalled: "Stalled", mediafiles.SlotFailed: "Failed", mediafiles.SlotImporting: "Importing",
	mediafiles.SlotNeedsADecision: "Needs a decision", mediafiles.SlotRejected: "Rejected", mediafiles.SlotInLibrary: "In library",
	mediafiles.SlotBetterWanted: "Better copy sought", mediafiles.SlotRemoved: "Removed",
}

// artworkLinks turns stored artwork ("poster:<cache name>") into links the
// web app can load (/api/v1/artwork/<cache name>); anything else is kept.
func artworkLinks(in []string) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if name, ok := strings.CutPrefix(a, "poster:"); ok && name != "" {
			a = "/api/v1/artwork/" + url.PathEscape(name)
		}
		out = append(out, a)
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (a *API) titleView(ctx context.Context, t mediafiles.Title) (TitleView, error) {
	db := a.s.Store.Read()
	v := TitleView{ID: t.ID, Kind: t.Kind, Name: t.Name, Year: t.Year, Overview: t.Overview, OriginalLanguage: t.OriginalLanguage, IDs: t.SourceIDs, Slots: []SlotView{},
		KeepUpToDate: t.KeepUpToDate, WhyAdded: t.WhyAdded, ReleaseGate: t.ReleaseGate, GateDelayDays: t.GateDelayDays,
		GateDriver: t.GateDriver, GateOpensAt: t.GateOpensAt, CinemaDate: t.CinemaDate, DigitalDate: t.DigitalDate,
		PhysicalDate: t.PhysicalDate, RefreshedAt: t.RefreshedAt, DroppedAt: t.DroppedAt, Artwork: artworkLinks(t.Artwork),
		Backdrop: t.Backdrop, Logo: t.Logo, Genres: nonNil(t.Genres), RuntimeMinutes: t.RuntimeMinutes, Rating: t.Rating, People: nonNil(t.People),
		AddedAt: t.CreatedAt, Tags: nonNil(t.Tags)}
	now := time.Now()
	var err error
	facts := factsOf(ctx)
	if facts != nil {
		v.SizeBytes = facts.sizes[t.ID]
		if c, ok := facts.copies[t.ID]; ok {
			v.Copy = &c
		}
		if at, ok := facts.activity[t.ID]; ok {
			v.LastActivity = &at
		}
	} else {
		if v.SizeBytes, err = titleSize(ctx, db, t.ID); err != nil {
			return v, err
		}
		if v.LastActivity, err = titleActivity(ctx, db, t.ID); err != nil {
			return v, err
		}
	}
	if t.Kind == "artist" {
		v.Stage, v.GateOpen = t.Status, true
		v.GateReason = "Each album is wanted once it is out (its release date has passed)."
		if !t.KeepUpToDate {
			v.GateReason += " Keep up to date is off for the artist, so HolyArrs does not look for it on its own."
		}
	} else if t.Kind == "show" {
		v.Stage, v.GateOpen = t.Status, true
		v.GateReason = "Each episode is wanted once it has aired."
		if !t.KeepUpToDate {
			v.GateReason += " Keep up to date is off for the show, so HolyArrs does not look for it on its own."
		}
	} else {
		v.Stage = library.Stage(library.DatesOf(t), now)
		v.GateOpen = library.GateOpen(t.GateDriver, t.GateOpensAt, now)
		v.GateReason = gateReason(t, v.GateOpen)
	}
	v.Versions = []VersionView{}
	var vs []mediafiles.Version
	if facts != nil {
		vs = facts.versions[t.ID]
	} else if vs, err = database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? ORDER BY is_main DESC, id", t.ID); err != nil {
		return v, err
	}
	for _, ver := range vs {
		v.Versions = append(v.Versions, versionView(ctx, db, ver))
	}
	if len(v.Versions) > 0 {
		m := v.Versions[0]
		v.LibraryFolderID, v.PickRulesID, v.FolderPath, v.PickRulesName = m.LibraryFolderID, m.PickRulesID, m.FolderPath, m.PickRulesName
	}
	batch := batchOf(ctx)
	titleAsks := func() ([]requests.View, error) {
		if batch != nil {
			return batch.asksFor(t.ID), nil
		}
		return a.s.Asks.TitleAsks(ctx, t.ID)
	}
	if t.Kind == "show" {
		if v.Show, err = a.showInfo(ctx, t); err != nil {
			return v, err
		}
		v.Asks, err = titleAsks()
		return trimForViewer(ctx, v), err
	}
	if t.Kind == "artist" {
		if v.Artist, err = a.artistInfo(ctx, t); err != nil {
			return v, err
		}
		v.Asks, err = titleAsks()
		return trimForViewer(ctx, v), err
	}
	var slots []mediafiles.Slot
	if batch != nil {
		slots = batch.slots[t.ID]
	} else if slots, err = database.Select[mediafiles.Slot](ctx, db, "slots",
		"item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", t.ID); err != nil {
		return v, err
	}
	for _, s := range slots {
		var ver mediafiles.Version
		if batch != nil {
			ver = batch.labels[s.VersionID]
		} else {
			_ = database.Get(ctx, db, "versions", s.VersionID, &ver)
		}
		sv := SlotView{ID: s.ID, Version: ver.Label, VersionID: ver.ID, State: string(s.State), StateLabel: StateLabels[s.State], DownloadID: s.ActiveDownloadID}
		if s.LastReason != nil {
			sv.Reason = *s.LastReason
		}
		if s.ActiveDownloadID != nil {
			var d domain.Download
			var found bool
			if batch != nil {
				d, found = batch.downloads[*s.ActiveDownloadID]
			} else {
				found = database.Get(ctx, db, "downloads", *s.ActiveDownloadID, &d) == nil
			}
			if found {
				dv := downloads.DownloadViewOf(d)
				sv.Download = &dv
			}
		}
		if s.FileID != nil {
			var f mediafiles.File
			var found bool
			if batch != nil {
				f, found = batch.files[*s.FileID]
			} else {
				found = database.Get(ctx, db, "files", *s.FileID, &f) == nil
			}
			if found {
				sv.File = &f
			}
		}
		v.Slots = append(v.Slots, sv)
	}
	if v.Asks, err = titleAsks(); err != nil {
		return v, err
	}
	return trimForViewer(ctx, v), nil
}

// gateReason says in one sentence why a movie is or is not searched yet.
func gateReason(t mediafiles.Title, open bool) string {
	date := func(at *time.Time) string { return at.Format("2006-01-02") }
	var when string
	switch t.GateDriver {
	case "announced":
		when = "the Release gate is set to announced, so searching may start any time"
	case "cinema":
		when = "the cinema date"
	case "digital":
		when = "the digital release date"
	case "physical":
		when = "the disc release date"
	case "cinema_plus_90":
		when = "90 days after the cinema date (no digital or disc date yet)"
	}
	var s string
	switch {
	case t.GateDriver == "announced":
		s = "The gate is open: " + when + "."
	case t.GateOpensAt == nil:
		s = "The gate is closed: there are no release dates yet, so HolyArrs waits until TMDB has one."
	case open:
		s = "The gate is open since " + date(t.GateOpensAt) + " (" + when + ")."
	default:
		s = "The gate is closed until " + date(t.GateOpensAt) + " (" + when + ")."
	}
	if t.GateDelayDays > 0 && t.GateDriver != "announced" && t.GateOpensAt != nil {
		s = strings.TrimSuffix(s, ".") + ", plus " + strconv.Itoa(t.GateDelayDays) + " days of delay."
	}
	if !t.KeepUpToDate {
		s += " Keep up to date is off, so HolyArrs does not look for it on its own."
	}
	return s
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
	ctx, err := withTitleFacts(withLookupMemo(r.Context()), a.s.Store.Read())
	if err == nil && len(ts) > 0 {
		ctx, err = a.withTitleBatch(ctx)
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
			v.Artist.Bio = a.keptArtistBio(r.Context(), v.Artist.MBID, t.Name)
		}
		if !manage {
			v.Copy = nil
		}
		out = append(out, v)
	}
	webserver.WriteTaggedJSON(w, r, map[string]any{"titles": out})
}

func (a *API) titleID(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var t mediafiles.Title
	if err == nil {
		err = database.Get(r.Context(), a.s.Store.Read(), "titles", id, &t)
	}
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such Title")
		return t, false
	}
	return t, true
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
	page, err := loadEventPage(r.Context(), a.s.Store.Read(), appendEvent(es, filtered))
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
			v.TitleName = page.titleName(e)
			v.Episode = page.episode(e)
			v.Version = page.version(e)
		}
		out = append(out, v)
	}
	resp := map[string]any{"events": out, "total": total, "limit": f.Limit, "offset": f.Offset}
	if filtered != nil {
		resp["episode"] = page.episode(*filtered)
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
		"music_covers": a.tplPreloadStatus(),
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
