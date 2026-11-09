package music

import (
	"context"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// Services are the program services the Music routes use, named as the
// old api.Server fields.
type Services struct {
	Store *database.Store
	// Packages are the kept artist packages; nil: none.
	Packages *PackageSync
	Queue    *jobs.Queue
	// Library and ImageBase are read on each use (tests swap them).
	Library func() *library.Service
	Live    *webserver.LiveHub
	Parts   *parts.Control
	Asks    *requests.Service
	// Trailers keeps trailer addresses and the relay the artist videos use.
	Trailers *metadata.Trailers
	// ImageBase is the outside picture address; MusicControl reads the
	// server's music switch; ArtistVideos lists an artist's music videos.
	ImageBase    func() string
	MusicControl func(ctx context.Context) (metadata.Control, error)
	ArtistVideos func(ctx context.Context, mbid string) ([]metadata.ArtistVideo, error)

	// Funcs owned by other modules (shows), filled by the caller.

	// SlotOfTitle says a track item belongs to the title.
	SlotOfTitle func(ctx context.Context, itemID, titleID int64) bool
	// SeasonLabel names a part's title and season ("S01").
	SeasonLabel func(ctx context.Context, partID int64) (int64, string, string)

	// Funcs owned by other modules, filled by the caller.

	PartIsOn         func(p parts.Part) bool
	KidMax           func(r *http.Request) string
	Person           func(r *http.Request) requests.Person
	OpenArtistAsks   func(ctx context.Context, mbid string, me int64) ([]requests.AskBrief, error)
	ArtistVideosOn   func() bool
	MusicFresh       func(row string, c MusicCard) *engine.Fresh
	ExploreCaching   func(w http.ResponseWriter, r *http.Request, cache string)
	TitleID          func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	TitleView        func(ctx context.Context, t mediafiles.Title) (any, error)
	PublishTitleView func(v any)
	// LibraryError answers the library's refusals with a status and a reason.
	LibraryError func(w http.ResponseWriter, err error)
	// Limit spends one of the person's Explore lookups; false: over budget.
	Limit func(userID int64, now time.Time) bool
	// TrackRoutes registers the album track routes (their Slot views are api's).
	TrackRoutes func(m *http.ServeMux)
}

// Handlers serves the Music routes. Deps points at the caller's flags and
// metadata calls (tests change them after building), Services holds the
// program services. Its state fields are private and zero-value ready.
type Handlers struct {
	*Deps
	Services
	pics    picRecords
	side    sideStore
	artWarm artWarmer

	music        musicTrendCache
	rising       risingCache
	musicx       musicExplore
	exploreCache exploreState
	genreArt     GenreArtCache
	artistAsked  artistAsked
	ahead        musicAhead
	musicPre     musicPreload
	ctl          musicControl
	artistVideos artistVideoCache
	// listSeen is each region's last world-list shelves signature
	// (music_lists_v1), guarded by exploreCache.cursorMu.
	listSeen map[string]string
	lookups  inFlight
}

// Routes registers no routes: the program registers the Music routes.
func (h *Handlers) Routes(m *http.ServeMux) {}

func (h *Handlers) lib() *library.Service {
	if h.Library == nil {
		return nil
	}
	return h.Library()
}

func (h *Handlers) imageBase() string {
	if h.ImageBase == nil {
		return ""
	}
	return h.ImageBase()
}
