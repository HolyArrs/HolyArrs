package discover

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// Deps is what the Explore routes need: services and core types, and funcs
// for anything a higher module owns (t14 split design, section 4). Filled
// as the routes move out of internal/api.
type Deps struct {
	Store     *database.Store
	Metadata  metadata.Provider
	Secrets   *secrets.Vault
	TraktBase string
	// Cache is the Explore row and details cache shared with api.
	Cache *Cache
	// KidMax is the signed-in person's age-rating limit ("" for none).
	KidMax         func(*http.Request) string
	UserID         func(*http.Request) int64
	Actor          func(*http.Request) domain.Actor
	PartOn         func(parts.Part) bool
	TitleAllowed   func(*http.Request, mediafiles.Title) bool
	LibraryRating  func(ctx context.Context, kind, tmdbID string) (string, bool)
	KidItems       func(ctx context.Context, max string, items []metadata.DiscoverItem, n int) []metadata.DiscoverItem
	CardFresh      func(ctx context.Context, it metadata.DiscoverItem, titleID int64) (*engine.Fresh, error)
	WithFreshRow   func(ctx context.Context, row string) context.Context
	OpenAsk        func(ctx context.Context, tmdbID string, me int64) (*requests.AskBrief, error)
	OpenShowAsk    func(ctx context.Context, ids map[string]string, me int64) (*requests.AskBrief, error)
	FillCardArt    func(r *http.Request, items []ItemView)
	QueueCardItems func(ctx context.Context, items []engine.Item)
	// ServiceTop gives the metadata server's per-service top list reader
	// (nil when there is none); ServiceTopCache and WatchOn are api's caches.
	ServiceTop      func() ServiceTopFetch
	ServiceTopCache *ServiceTopCache
	WatchOn         *WatchOn
	FillTrailerKeys func(r *http.Request, items []ItemView)
	// KidKnownFor trims a person's known-for titles to a Kid's limit;
	// LibraryMovieAllowed says whether a Library movie fits that limit.
	KidKnownFor         func(ctx context.Context, max string, p *metadata.Person)
	LibraryMovieAllowed func(ctx context.Context, max, tmdbID string) bool
	// LiveSource lists Explore's live TMDB lists (the trending shelf).
	LiveSource func(ctx context.Context) Lister
}

// ServiceTopFetch reads one region's per-service top lists.
type ServiceTopFetch func(ctx context.Context, region string) (metadata.ServiceTop, error)

// Lister answers one Explore list request.
type Lister interface {
	List(ctx context.Context, q engine.Request) ([]engine.Item, error)
}

// Handlers serves the Explore routes. Its state fields are private and
// zero-value ready, so &Handlers{Deps: d} works.
type Handlers struct {
	Deps
}

// ItemView is one discover item with what the Library knows of it.
type ItemView struct {
	metadata.DiscoverItem
	InLibrary bool               `json:"in_library"`
	TitleID   int64              `json:"title_id,omitempty"`
	Ask       *requests.AskBrief `json:"ask"`
	// Fresh is the card's one freshness strip (New Episode, Top 10...).
	Fresh *engine.Fresh `json:"fresh,omitempty"`
	// TrailerKey is the title's YouTube trailer when HolyArrs already knows
	// it (Explore only; never looked up for this answer).
	TrailerKey string `json:"trailer_key,omitempty"`
	// Streaming is the member's service the title is on (kept lists only).
	Streaming *StreamingMark `json:"streaming,omitempty"`
	// The wide card's picture (card_art, card_logo, art_has_title), from
	// the card_art table only.
	metadata.CardArt
}

// Routes registers the Explore routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}", h.metadataDetails)
	m.HandleFunc("GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}", h.metadataSeason)
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}/more-from", h.metadataMoreFrom)
	reviews := h.reviewHandlers()
	reviews.Routes(m)
	m.HandleFunc("GET /api/v1/discover", h.discoverRow)
	m.HandleFunc("GET /api/v1/watch-services", h.watchServices)
	m.HandleFunc("GET /api/v1/explore/service-top", h.exploreServiceTop)
}

// TokenRoutes registers the Trakt VIP token setting (api registers it only
// when the vault and the Trakt feature are on).
func (h *Handlers) TokenRoutes(m *http.ServeMux) { h.reviewHandlers().TokenRoutes(m) }
