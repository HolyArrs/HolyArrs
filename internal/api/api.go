// Package api serves the versioned HTTP API under /api/v1, the live stream
// at /live and the web app at /.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/CoderLuii/holyarrs/internal/music"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/alerts"
	"github.com/CoderLuii/holyarrs/internal/applog"
	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/backup"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/health"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/lists"
	"github.com/CoderLuii/holyarrs/internal/live"
	"github.com/CoderLuii/holyarrs/internal/mediaserver"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/migrate"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/subtitles"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
	"github.com/CoderLuii/holyarrs/web"
)

// Server is what the API needs.
type Server struct {
	Store    *store.Store
	Queue    *jobs.Queue
	Live     *live.Hub
	Metadata metadata.Provider
	Version  string
	Started  time.Time
	// Auth guards every route but the health check and the sign-in page.
	// Nil serves without sign-in (tests only).
	Auth *auth.Auth
	// Backups serves the backup routes when set; Restart is called after a
	// restore is staged.
	Backups *backup.Manager
	Restart func()
	// Secrets stores the encrypted settings.
	Secrets *secrets.Vault
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
	// ExtraArt and ExtraRatings read fanart.tv artwork and outside ratings
	// from the metadata server; nil (or a source off there) shows neither.
	ExtraArt     func(ctx context.Context, kind, id string) (metadata.ServerArt, error)
	// ImageBase is the metadata server's picture origin ("<server>/v1/img");
	// its sharp artist photo is the first choice for every artist picture.
	// Empty skips it.
	ImageBase string
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
	// Library runs adding, editing and refreshing; New makes one when nil.
	Library *library.Service
	// Lists reads Outside lists; New makes one without keys when nil.
	Lists *lists.Service
	// Alerts sends Discord and email Alerts; New makes one when Secrets is set.
	Alerts *alerts.Sender
	// Asks runs the Wishlist; New makes one when nil.
	Asks *asks.Service
	// Downloads follows Downloaders and Downloads; made on first use when nil.
	Downloads *downloader.Manager
	// Search runs searches, hand choices and the Held and Turned down
	// lists; New makes one (choices queue a grab Job) when nil.
	Search *source.Searcher
	// Health runs the health checks; New makes one when nil.
	Health *health.Engine
	// Media tells media servers about changed folders; New makes one when
	// Secrets is set.
	Media *mediaserver.Linker
	// Subtitles runs subtitle work; New makes one when nil.
	Subtitles *subtitles.Service
	// States holds the state of every supervised stage (may be nil).
	States *supervisor.Registry
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
	Log *applog.Log
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
	// listSeen is each region's last world-list shelves signature
	// (music_lists_v1), guarded by musicv2.cursorMu.
	listSeen map[string]string
	// lookups counts each person's music lookups in flight.
	lookups inFlight
	// confirm holds the two-step controls' confirmation ids.
	confirm confirms
	// discover caches Explore rows and details pages and limits each person.
	discover discoverState
	// rated keeps TMDB ratings looked up for Kid limits.
	rated ratingState
	// status keeps the Settings status answer for 30 seconds.
	status statusCache
	// serviceTop keeps the per-service top lists for 12 hours.
	serviceTop serviceTopCache
	// watchOn keeps each title's TMDB watch providers per region a day.
	watchOn watchOnCache
	// music keeps the music trending lists for an hour.
	music musicTrendCache
	// musicx keeps artist genres and similar artists for Explore → Music.
	musicx musicExplore
	// musicAns keeps each member's Explore → Music answers a minute.
	musicAns musicAnswers
	// musicv2 holds the music_rows_v2 cache layers.
	musicv2 musicV2State
	// genreArt keeps genre tile pictures.
	genreArt musicGenreArtCache
	// signin keeps the sign-in page's art and its per-address budget.
	signin signinArtState
	// signinSeed keeps the first sign-in slide's tiny thumbnails.
	signinSeed signinSeedState
	// explore runs the Explore pool job and keeps its lists and pages.
	explore   exploreState
	genreRows genreRowCache
	// trailers keeps resolved trailer addresses and each person's budget.
	trailers trailerState
	// trailerRelay holds the trailer streams HolyArrs relays (pass-through).
	trailerRelay trailerProxy
	// trailerKeys keeps each title's YouTube key (also in lookup_cache).
	trailerKeys trailerKeyCache
	// ahead runs music fetch-ahead; ctl keeps the server's switch.
	ahead musicAhead
	artWarm artWarmer
	// musicPre stores every engine page's covers ahead (musicpreload.go).
	musicPre musicPreload
	ctl      musicControl
	// artistVideos keeps each artist's music videos (see music_video.go).
	artistVideos artistVideoCache
}

// New makes the API.
func New(s Server) *API {
	if s.Started.IsZero() {
		s.Started = time.Now()
	}
	if s.Library == nil {
		s.Library = &library.Service{Store: s.Store, Queue: s.Queue, Metadata: s.Metadata, Live: s.Live}
	}
	if s.Lists == nil {
		s.Lists = &lists.Service{Store: s.Store, Queue: s.Queue, Library: s.Library, Metadata: s.Metadata}
	}
	if s.Alerts == nil && s.Secrets != nil {
		s.Alerts = &alerts.Sender{Store: s.Store, Vault: s.Secrets}
	}
	if s.Asks == nil {
		s.Asks = &asks.Service{Store: s.Store, Library: s.Library, Metadata: s.Metadata, Live: s.Live}
		if s.Alerts != nil {
			s.Asks.Alerts = s.Alerts
		}
	}
	if s.Health == nil {
		s.Health = &health.Engine{Store: s.Store, Version: s.Version}
	}
	if s.Media == nil && s.Secrets != nil {
		s.Media = &mediaserver.Linker{Store: s.Store, Vault: s.Secrets}
	}
	if s.Subtitles == nil {
		s.Subtitles = newSubtitles(s)
	}
	if s.Search == nil {
		q := s.Queue
		var pub source.Publisher
		if s.Live != nil {
			pub = s.Live // a nil hub must stay a nil Publisher
		}
		s.Search = &source.Searcher{Store: s.Store, Live: pub, Secrets: s.Secrets, Queue: q,
			OnChosen: func(ctx context.Context, slotID, releaseID int64) {
				if q == nil {
					return
				}
				if _, _, err := q.Enqueue(ctx, downloader.KindGrab, &domain.Subject{Type: "slot", ID: slotID}, map[string]any{"release_id": releaseID},
					"grab:"+itoa(slotID), domain.System); err != nil {
					log.Printf("queue grab: %v", err)
				}
			}}
	}
	return &API{s: s}
}

// actor is the signed-in person; without sign-in (tests) it is the owner.
func actor(r *http.Request) domain.Actor {
	if _, ok := auth.From(r.Context()); ok {
		return auth.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

// Handler returns every route.
func (a *API) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/titles", a.listTitles)
	m.HandleFunc("GET /api/v1/library/search", a.librarySearch)
	m.HandleFunc("GET /api/v1/search/suggest", a.searchSuggest)
	m.HandleFunc("GET /api/v1/library-summary", a.librarySummary)
	m.HandleFunc("GET /api/v1/home", a.home)
	m.HandleFunc("POST /api/v1/titles", a.addTitle)
	m.HandleFunc("GET /api/v1/titles/lookup", a.lookupTitles)
	m.HandleFunc("GET /api/v1/titles/{id}", a.getTitle)
	m.HandleFunc("PUT /api/v1/titles/{id}", a.editTitle)
	m.HandleFunc("POST /api/v1/titles/{id}/move", a.moveTitle)
	m.HandleFunc("POST /api/v1/titles/{id}/versions", a.addVersion)
	m.HandleFunc("PUT /api/v1/titles/{id}/versions/{vid}", a.editVersion)
	m.HandleFunc("DELETE /api/v1/titles/{id}/versions/{vid}", a.deleteVersion)
	m.HandleFunc("POST /api/v1/titles/{id}/refresh", a.refreshTitle)
	m.HandleFunc("POST /api/v1/titles/refresh", a.refreshTitles)
	m.HandleFunc("GET /api/v1/calendar", a.calendar)
	m.HandleFunc("GET /api/v1/artwork/{name}", a.artwork)
	m.HandleFunc("GET /api/v1/artwork/tmdb/{size}/{file}", a.tmdbArtwork)
	m.HandleFunc("GET /api/v1/settings/status", a.settingsStatusRoute)
	m.HandleFunc("GET /api/v1/settings/title-defaults", a.getTitleDefaults)
	m.HandleFunc("PUT /api/v1/settings/title-defaults", a.putTitleDefaults)
	m.HandleFunc("GET /api/v1/pick-rules", a.listPickRules)
	m.HandleFunc("POST /api/v1/pick-rules", a.addPickRules)
	m.HandleFunc("GET /api/v1/pick-rules/{id}", a.getPickRules)
	m.HandleFunc("PUT /api/v1/pick-rules/{id}", a.editPickRules)
	m.HandleFunc("DELETE /api/v1/pick-rules/{id}", a.deletePickRules)
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
	m.HandleFunc("GET /api/v1/outside-lists", a.listOutsideLists)
	m.HandleFunc("POST /api/v1/outside-lists", a.addOutsideList)
	m.HandleFunc("PUT /api/v1/outside-lists/{id}", a.editOutsideList)
	m.HandleFunc("DELETE /api/v1/outside-lists/{id}", a.deleteOutsideList)
	m.HandleFunc("POST /api/v1/outside-lists/{id}/sync", a.syncOutsideList)
	m.HandleFunc("POST /api/v1/titles/{id}/search", a.searchTitle)
	m.HandleFunc("GET /api/v1/titles/{id}/releases", a.titleReleases)
	m.HandleFunc("POST /api/v1/releases/{id}/choose", a.chooseRelease)
	m.HandleFunc("POST /api/v1/releases/{id}/turn-down", a.turnDownRelease)
	m.HandleFunc("GET /api/v1/held", a.listState("held"))
	m.HandleFunc("DELETE /api/v1/held", a.removeFrom("held", false))
	m.HandleFunc("DELETE /api/v1/held/{id}", a.removeFrom("held", true))
	m.HandleFunc("GET /api/v1/turned-down", a.listState("turned_down"))
	m.HandleFunc("DELETE /api/v1/turned-down", a.removeFrom("turned_down", false))
	m.HandleFunc("DELETE /api/v1/turned-down/{id}", a.removeFrom("turned_down", true))
	m.HandleFunc("POST /api/v1/search/missing", a.bulkSearch("missing"))
	m.HandleFunc("POST /api/v1/search/better", a.bulkSearch("better"))
	m.HandleFunc("POST /api/v1/search/feed-check", a.feedCheckNow)
	m.HandleFunc("GET /api/v1/searches", a.searches)
	m.HandleFunc("GET /api/v1/settings/search", a.getSearchSettings)
	m.HandleFunc("PUT /api/v1/settings/search", a.putSearchSettings)
	m.HandleFunc("GET /api/v1/downloads", a.listDownloads)
	m.HandleFunc("GET /api/v1/imports", a.listImports)
	m.HandleFunc("GET /api/v1/events", a.listEvents)
	m.HandleFunc("GET /api/v1/jobs", a.listJobs)
	a.peopleRoutes(m)
	a.copyRoutes(m)
	m.HandleFunc("GET /api/v1/collections", a.listCollections)
	m.HandleFunc("GET /api/v1/collections/{id}", a.getCollection)
	m.HandleFunc("POST /api/v1/collections/{id}/remove/preview", a.previewCollectionRemoval)
	m.HandleFunc("POST /api/v1/collections/{id}/remove", a.removeCollection)
	m.HandleFunc("GET /api/v1/library/copies", a.listLibraryCopies)
	m.HandleFunc("POST /api/v1/library/copies/search/preview", a.previewLibraryCopySearch)
	m.HandleFunc("POST /api/v1/library/copies/search", a.searchLibraryCopies)
	a.sourceRoutes(m)
	a.downloaderRoutes(m)
	a.organiseRoutes(m)
	a.healthRoutes(m)
	a.mediaServerRoutes(m)
	a.feedRoutes(m)
	a.showRoutes(m)
	a.musicRoutes(m)
	a.musicPageRoutes(m)
	a.subtitleRoutes(m)
	a.importSecretsRoutes(m)
	a.thisAppRoutes(m)
	a.dataPackRoutes(m)
	a.scheduleRoutes(m)
	m.HandleFunc("GET /api/v1/metadata/search", a.searchMetadata)
	m.HandleFunc("GET /api/v1/metadata/health", a.metadataHealth)
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}", a.metadataDetails)
	m.HandleFunc("GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}", a.metadataSeason)
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}/more-from", a.metadataMoreFrom)
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}/reviews", a.metadataReviews)
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}/anime", a.metadataAnime)
	m.HandleFunc("GET /api/v1/trailer/batch", a.trailerBatch)
	m.HandleFunc("GET /api/v1/trailer/{kind}/{tmdb_id}", a.trailerStream)
	m.HandleFunc("GET /api/v1/trailer/stream/{id}/manifest.m3u8", a.trailerManifest)
	m.HandleFunc("GET /api/v1/trailer/stream/{id}/seg/{n}", a.trailerSegment)
	m.HandleFunc("GET /api/v1/settings/ui", a.getUISettings)
	a.signinArtRoutes(m)
	m.HandleFunc("PUT /api/v1/settings/ui", a.putUISettings)
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
	m.HandleFunc("GET /api/v1/explore/title-stats", a.exploreTitleStats)
	m.HandleFunc("GET /api/v1/music-trending", a.musicTrending)
	m.HandleFunc("GET /api/v1/explore/music", a.exploreMusic)
	m.HandleFunc("GET /api/v1/music-trending/art/{kind}/{mbid}", a.musicTrendArt)
	m.HandleFunc("GET /api/v1/music/genre/{slug}", a.musicGenrePage)
	m.HandleFunc("GET /api/v1/music/browse/{kind}/{slug}", a.musicBrowsePage)
	m.HandleFunc("GET /api/v1/music/browse-tiles", a.musicBrowseTiles)
	m.HandleFunc("GET /api/v1/music/genre-art/{slug}", a.musicGenreArt)
	m.HandleFunc("GET /api/v1/extras/{kind}/{id}", a.titleExtras)
	m.HandleFunc("GET /api/v1/wiki/{kind}/{id}", a.wikiFacts)
	if a.s.Secrets != nil {
		m.HandleFunc("GET /api/v1/settings/trakt-key", a.getTraktKey)
		m.HandleFunc("PUT /api/v1/settings/trakt-key", a.putTraktKey)
		m.HandleFunc("GET /api/v1/settings/trakt-token", a.getTraktToken)
		m.HandleFunc("PUT /api/v1/settings/trakt-token", a.putTraktToken)
		m.HandleFunc("GET /api/v1/settings/tvdb-key", a.getTVDBKey)
		m.HandleFunc("PUT /api/v1/settings/tvdb-key", a.putTVDBKey)
	}
	a.askRoutes(m)
	a.askedByRoutes(m)
	a.followRoutes(m)
	a.controlRoutes(m)
	a.migrateRoutes(m)
	a.moveRoutes(m)
	m.HandleFunc("GET /api/v1/openapi.json", a.openapi)
	m.HandleFunc("GET /api/v1/health", a.health)
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	if a.s.Live != nil {
		m.Handle("GET /live", a.s.Live)
	}
	m.Handle("GET /", web.Handler())
	if a.s.Backups != nil {
		a.s.Backups.Routes(m, a.s.Restart)
	}
	if a.s.Auth != nil {
		a.s.Auth.Routes(m)
		return secure(a.s.Auth.Middleware(wrapMux(a.forgetStatusOnWrite(a.kidGate(m, a.partGate(m, auth.Gate(m)))))))
	}
	return secure(wrapMux(a.forgetStatusOnWrite(a.partGate(m, m))))
}

// wrapMux lets tests record which routes the mux served.
var wrapMux = func(h http.Handler) http.Handler { return h }

func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Content-Security-Policy", web.CSP(nil))
		h.ServeHTTP(w, r)
	})
}

// healthz is open to everyone: it says only whether the database answers.
func (a *API) healthz(w http.ResponseWriter, _ *http.Request) {
	if _, err := a.s.Store.Version(); err != nil {
		http.Error(w, "database not answering\n", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func internal(w http.ResponseWriter, err error) {
	log.Printf("api: %v", err)
	fail(w, http.StatusInternalServerError, "something went wrong; see the log")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// SlotView is a Slot as the page shows it.
type SlotView struct {
	ID         int64        `json:"id"`
	Version    string       `json:"version"`
	VersionID  int64        `json:"version_id"`
	State      string       `json:"state"`
	StateLabel string       `json:"state_label"`
	Reason     string       `json:"reason,omitempty"`
	File       *domain.File `json:"file,omitempty"`
	DownloadID *int64       `json:"active_download_id,omitempty"`
	// Download is the active (or last) Download in plain words.
	Download *DownloadView `json:"download,omitempty"`
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
	// AddedAt is when the title joined the Library; SizeBytes is the sum of
	// every file kept for it, across all Versions (Library table columns).
	AddedAt   time.Time `json:"added_at"`
	SizeBytes int64     `json:"size_bytes"`
	// LastActivity is the newest grab or import for the title, if any.
	LastActivity *time.Time `json:"last_activity,omitempty"`
	// Versions are the wanted copies (main first), such as HD and 4K.
	Versions []VersionView `json:"versions"`
	Slots    []SlotView    `json:"slots"`
	// Asks are the Wishlist asks that led to it: who asked and who decided.
	Asks []asks.View `json:"asks"`
	// Show holds a show's choices and season summaries (shows only; their
	// episodes and Slots come from /titles/{id}/episodes, so Slots is empty).
	Show *ShowInfo `json:"show,omitempty"`
	// Artist holds an artist's choices and counts (artists only; albums,
	// Pressings and tracks come from /titles/{id}/albums and /albums/{id}).
	Artist *ArtistInfo `json:"artist,omitempty"`
}

// ShowInfo is what a show's page needs besides the shared Title fields.
type ShowInfo struct {
	DetailsSource string                  `json:"details_source"`
	EpisodeOrder  string                  `json:"episode_order"`
	SeasonChoice  string                  `json:"season_choice,omitempty"`
	NewSeasons    string                  `json:"new_seasons"`
	Status        string                  `json:"status"`
	Network       string                  `json:"network,omitempty"`
	AirTime       string                  `json:"air_time,omitempty"`
	TimeZone      string                  `json:"time_zone,omitempty"`
	Seasons       []library.SeasonSummary `json:"seasons"`
	Episodes      int                     `json:"episodes"`
	Missing       int                     `json:"missing"`
	NotOutYet     int                     `json:"not_out_yet"`
	InLibrary     int                     `json:"in_library"`
	BetterWanted  int                     `json:"better_wanted"`
	Downloading   int                     `json:"downloading"`
	Failed        int                     `json:"failed"`
	NextAirDate   *time.Time              `json:"next_air_date,omitempty"`
	// SeasonCount counts numbered seasons (specials left out); LastAirDate
	// is the most recent episode that has aired.
	SeasonCount int        `json:"season_count"`
	LastAirDate *time.Time `json:"last_air_date,omitempty"`
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (a *API) showInfo(ctx context.Context, t domain.Title) (*ShowInfo, error) {
	ss, err := library.Seasons(ctx, a.s.Store.Read(), t.ID, time.Now())
	if err != nil {
		return nil, err
	}
	si := &ShowInfo{DetailsSource: t.MetadataSource, EpisodeOrder: strOr(t.EpisodeOrder), SeasonChoice: strOr(t.SeasonChoice), NewSeasons: t.NewPartsPolicy,
		Status: t.Status, Network: strOr(t.Network), AirTime: strOr(t.AirTime), TimeZone: strOr(t.TimeZone), Seasons: ss}
	for _, s := range ss {
		if s.Number > 0 {
			si.SeasonCount++
		}
		si.Episodes += s.Episodes
		si.Missing += s.Missing
		si.NotOutYet += s.NotOutYet
		si.InLibrary += s.InLibrary
		si.BetterWanted += s.BetterWanted
		si.Downloading += s.Downloading
		si.Failed += s.Failed
		if s.NextAirDate != nil && (si.NextAirDate == nil || s.NextAirDate.Before(*si.NextAirDate)) {
			si.NextAirDate = s.NextAirDate
		}
	}
	if si.LastAirDate, err = lastAired(ctx, a.s.Store.Read(), t.ID, time.Now()); err != nil {
		return nil, err
	}
	return si, nil
}

// StateLabels are the words people see for Slot states.
var StateLabels = map[domain.SlotState]string{
	domain.SlotNotOutYet: "Not out yet", domain.SlotWanted: "Missing", domain.SlotSearching: "Searching",
	domain.SlotHeld: "Held", domain.SlotChosen: "Chosen", domain.SlotDownloading: "Downloading",
	domain.SlotStalled: "Stalled", domain.SlotFailed: "Failed", domain.SlotImporting: "Importing",
	domain.SlotNeedsADecision: "Needs a decision", domain.SlotRejected: "Rejected", domain.SlotInLibrary: "In library",
	domain.SlotBetterWanted: "Better copy sought", domain.SlotRemoved: "Removed",
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

func (a *API) titleView(ctx context.Context, t domain.Title) (TitleView, error) {
	db := a.s.Store.Read()
	v := TitleView{ID: t.ID, Kind: t.Kind, Name: t.Name, Year: t.Year, Overview: t.Overview, OriginalLanguage: t.OriginalLanguage, IDs: t.SourceIDs, Slots: []SlotView{},
		KeepUpToDate: t.KeepUpToDate, WhyAdded: t.WhyAdded, ReleaseGate: t.ReleaseGate, GateDelayDays: t.GateDelayDays,
		GateDriver: t.GateDriver, GateOpensAt: t.GateOpensAt, CinemaDate: t.CinemaDate, DigitalDate: t.DigitalDate,
		PhysicalDate: t.PhysicalDate, RefreshedAt: t.RefreshedAt, DroppedAt: t.DroppedAt, Artwork: artworkLinks(t.Artwork),
		Backdrop: t.Backdrop, Logo: t.Logo, Genres: nonNil(t.Genres), RuntimeMinutes: t.RuntimeMinutes, Rating: t.Rating, People: nonNil(t.People),
		AddedAt: t.CreatedAt}
	now := time.Now()
	var err error
	facts := factsOf(ctx)
	if facts != nil {
		v.SizeBytes = facts.sizes[t.ID]
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
	var vs []domain.Version
	if facts != nil {
		vs = facts.versions[t.ID]
	} else if vs, err = store.Select[domain.Version](ctx, db, "versions", "title_id = ? ORDER BY is_main DESC, id", t.ID); err != nil {
		return v, err
	}
	for _, ver := range vs {
		v.Versions = append(v.Versions, versionView(ctx, db, ver))
	}
	if len(v.Versions) > 0 {
		m := v.Versions[0]
		v.LibraryFolderID, v.PickRulesID, v.FolderPath, v.PickRulesName = m.LibraryFolderID, m.PickRulesID, m.FolderPath, m.PickRulesName
	}
	if t.Kind == "show" {
		if v.Show, err = a.showInfo(ctx, t); err != nil {
			return v, err
		}
		v.Asks, err = a.titleAsks(ctx, t.ID)
		return v, err
	}
	if t.Kind == "artist" {
		if v.Artist, err = a.artistInfo(ctx, t); err != nil {
			return v, err
		}
		v.Asks, err = a.titleAsks(ctx, t.ID)
		return v, err
	}
	slots, err := store.Select[domain.Slot](ctx, db, "slots",
		"item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", t.ID)
	if err != nil {
		return v, err
	}
	for _, s := range slots {
		var ver domain.Version
		_ = store.Get(ctx, db, "versions", s.VersionID, &ver)
		sv := SlotView{ID: s.ID, Version: ver.Label, VersionID: ver.ID, State: string(s.State), StateLabel: StateLabels[s.State], DownloadID: s.ActiveDownloadID}
		if s.LastReason != nil {
			sv.Reason = *s.LastReason
		}
		if s.ActiveDownloadID != nil {
			var d domain.Download
			if store.Get(ctx, db, "downloads", *s.ActiveDownloadID, &d) == nil {
				dv := downloadView(d)
				sv.Download = &dv
			}
		}
		if s.FileID != nil {
			var f domain.File
			if store.Get(ctx, db, "files", *s.FileID, &f) == nil {
				sv.File = &f
			}
		}
		v.Slots = append(v.Slots, sv)
	}
	if v.Asks, err = a.titleAsks(ctx, t.ID); err != nil {
		return v, err
	}
	return v, nil
}

// gateReason says in one sentence why a movie is or is not searched yet.
func gateReason(t domain.Title, open bool) string {
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
	ts, err := a.s.Store.SearchTitles(r.Context(), r.URL.Query().Get("q"))
	if errors.Is(err, store.ErrTitleSearchQuery) {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	out := []TitleView{}
	ctx, err := withTitleFacts(withLookupMemo(r.Context()), a.s.Store.Read())
	if err != nil {
		internal(w, err)
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
			internal(w, err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"titles": out})
}

func (a *API) titleID(w http.ResponseWriter, r *http.Request) (domain.Title, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var t domain.Title
	if err == nil {
		err = store.Get(r.Context(), a.s.Store.Read(), "titles", id, &t)
	}
	if err != nil {
		fail(w, http.StatusNotFound, "no such Title")
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
	ds, err := store.Select[domain.Download](r.Context(), a.s.Store.Read(), "downloads", "1=1 ORDER BY id DESC LIMIT ?", limit(r, 50))
	if err != nil {
		internal(w, err)
		return
	}
	if ds == nil {
		ds = []domain.Download{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"downloads": ds})
}

func (a *API) listImports(w http.ResponseWriter, r *http.Request) {
	is, err := store.Select[domain.Import](r.Context(), a.s.Store.Read(), "imports", "1=1 ORDER BY id DESC LIMIT ?", limit(r, 20))
	if err != nil {
		internal(w, err)
		return
	}
	if is == nil {
		is = []domain.Import{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"imports": is})
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
	f := store.EventFilter{Limit: limit(r, 50)}
	if v := q.Get("title"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			fail(w, http.StatusBadRequest, "title must be a movie or show id")
			return
		}
		f.TitleID = id
	}
	if v := q.Get("episode"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			fail(w, http.StatusBadRequest, "episode must be an episode id")
			return
		}
		var titleID int64
		if err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT title_id FROM items WHERE id = ? AND kind = 'episode'", id).Scan(&titleID); err != nil {
			fail(w, http.StatusBadRequest, "no such episode")
			return
		}
		f.TitleID, f.ItemIDs, f.SlotIDs = titleID, []int64{id}, []int64{}
		rows, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT id FROM slots WHERE item_id = ?", id)
		if err != nil {
			internal(w, err)
			return
		}
		for rows.Next() {
			var sid int64
			if err := rows.Scan(&sid); err != nil {
				rows.Close()
				internal(w, err)
				return
			}
			f.SlotIDs = append(f.SlotIDs, sid)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			internal(w, err)
			return
		}
	}
	if v := q.Get("album"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			fail(w, http.StatusBadRequest, "album must be an album id")
			return
		}
		var titleID int64
		if err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT title_id FROM parts WHERE id = ? AND kind = 'album'", id).Scan(&titleID); err != nil {
			fail(w, http.StatusBadRequest, "no such album")
			return
		}
		f.TitleID, f.PartID, f.ItemIDs, f.SlotIDs = titleID, id, []int64{}, []int64{}
		rows, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT i.id, s.id FROM items i LEFT JOIN slots s ON s.item_id = i.id WHERE i.part_id = ? AND i.kind = 'track'", id)
		if err != nil {
			internal(w, err)
			return
		}
		for rows.Next() {
			var iid int64
			var sid sql.NullInt64
			if err := rows.Scan(&iid, &sid); err != nil {
				rows.Close()
				internal(w, err)
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
			internal(w, err)
			return
		}
	}
	for _, k := range strings.Split(q.Get("kind"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			if len(k) > 60 || len(f.Kinds) >= 50 {
				fail(w, http.StatusBadRequest, "kind is a comma list of at most 50 Event kinds")
				return
			}
			f.Kinds = append(f.Kinds, k)
		}
	}
	if v := q.Get("from"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			fail(w, http.StatusBadRequest, "from must be a date like 2026-10-01")
			return
		}
		f.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			fail(w, http.StatusBadRequest, "to must be a date like 2026-10-31")
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
			fail(w, http.StatusBadRequest, "offset must be 0 or more")
			return
		}
		f.Offset = n
	}
	es, total, err := a.s.Store.Events(r.Context(), f)
	if err != nil {
		internal(w, err)
		return
	}
	// The filtered episode, named for the screen, is read with the page.
	var filtered *domain.Event
	if len(f.ItemIDs) == 1 {
		filtered = &domain.Event{TitleID: &f.TitleID, Data: map[string]any{"item_id": float64(f.ItemIDs[0])}}
	}
	page, err := loadEventPage(r.Context(), a.s.Store.Read(), appendEvent(es, filtered))
	if err != nil {
		internal(w, err)
		return
	}
	out := make([]EventView, 0, len(es))
	for _, e := range es {
		v := EventView{Event: e}
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
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	js, err := store.Select[domain.Job](r.Context(), a.s.Store.Read(), "jobs", "1=1 ORDER BY id DESC LIMIT ?", limit(r, 50))
	if err != nil {
		internal(w, err)
		return
	}
	if js == nil {
		js = []domain.Job{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": js})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": ok, "version": a.s.Version, "schema": ver, "uptime_s": int(time.Since(a.s.Started).Seconds()),
		"write_wait_p95_ms": float64(p95.Microseconds()) / 1000, "write_wait_samples": n, "writes": a.s.Store.WriteCount(),
		"sources": srcs, "downloaders": dls, "live_clients": clients, "url_base": a.s.URLBase,
		"music_covers": a.tplPreloadStatus(),
	})
}

func (a *API) searchMetadata(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" || len(q) > 200 {
		fail(w, http.StatusBadRequest, "send ?q=<name>, and &year=<year> if you know it")
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
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if ms == nil {
		ms = []metadata.Movie{}
	}
	ms = a.kidMovies(r.Context(), a.kidMax(r), ms)
	writeJSON(w, http.StatusOK, map[string]any{"movies": ms, "attribution": metadata.Attribution})
}
