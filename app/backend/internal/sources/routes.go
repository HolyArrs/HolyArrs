package sources

import (
	"context"
	"net/http"
	"strconv"
	"sync"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// Catalog is what the routes ask of the library: a Title from the request
// path and the plain-words names of Slots and Parts. The library sits below
// this package, so the program wires it in.
type Catalog interface {
	// TitleFromPath reads the {id} path value and answers 404 itself when
	// there is no such Title.
	TitleFromPath(w http.ResponseWriter, r *http.Request) (domain.Title, bool)
	EpisodeLabels(ctx context.Context, slotIDs []int64) string
	AlbumLabels(ctx context.Context, slotIDs []int64) string
	PartLabel(ctx context.Context, partID int64) (titleID int64, title, season, album string)
}

// OldApps is the old-app (Sonarr and Radarr style) side of connecting
// Prowlarr: its keys and its on switch live outside this package.
type OldApps interface {
	// CanMakeKeys is false when the app runs without sign-in, so no key can be made.
	CanMakeKeys() bool
	// KeySides says which sides (sonarr, radarr) already have a key.
	KeySides(ctx context.Context) (map[string]bool, error)
	MakeKey(r *http.Request, side string) (string, error)
	DeleteKey(r *http.Request, side string) error
	On(ctx context.Context) bool
	SetOn(ctx context.Context, by domain.Actor, on bool) error
}

// Handlers serves the Source, search and bridge routes.
type Handlers struct {
	Store *database.Store
	// Vault is nil when the app runs without a key store.
	Vault *secrets.Vault
	// Search returns the searcher; it is looked up on each request because
	// it can be made after the routes are registered.
	Search func() *Searcher
	// Bridges is the connected Jackett and Prowlarr; nil makes one on first use.
	Bridges *BridgeService
	Queue   *jobs.Queue
	// Live is nil when nothing listens for live events.
	Live    Publisher
	Catalog Catalog
	OldApps OldApps
	// PartOn says whether a part of the app (Shows, Movies) is switched on.
	PartOn func(parts.Part) bool

	bridgeOnce sync.Once
	bridgeSvc  *BridgeService
}

// Routes registers every route of this module.
func (h *Handlers) Routes(r *webserver.Router) {
	h.sourceRoutes(r)
	h.searchRoutes(r)
	h.bridgeRoutes(r)
}

func (h *Handlers) sourceRoutes(r *webserver.Router) {
	r.Handle("GET /api/v1/sources", webserver.PermManageLibrary, http.HandlerFunc(h.listSources))
	r.Handle("POST /api/v1/sources", webserver.PermSettings, http.HandlerFunc(h.addSource))
	r.Handle("PUT /api/v1/sources/{id}", webserver.PermSettings, http.HandlerFunc(h.editSource))
	r.Handle("DELETE /api/v1/sources/{id}", webserver.PermSettings, http.HandlerFunc(h.deleteSource))
	r.Handle("POST /api/v1/sources/{id}/test", webserver.PermSettings, http.HandlerFunc(h.TestSource))
	r.Handle("POST /api/v1/sources/{id}/pause", webserver.PermSettings, http.HandlerFunc(h.pauseSource))
	r.Handle("POST /api/v1/sources/{id}/resume", webserver.PermSettings, http.HandlerFunc(h.resumeSource))
	r.Handle("GET /api/v1/sources/{id}/stats", webserver.PermManageLibrary, http.HandlerFunc(h.sourceStats))
	r.Handle("POST /api/v1/sources/{id}/try", webserver.PermSettings, http.HandlerFunc(h.trySource))
	r.Handle("POST /api/v1/sources/check", webserver.PermSettings, http.HandlerFunc(h.checkSources))
	r.Handle("GET /api/v1/titles/{id}/search-outcome", webserver.NeedSignedIn, http.HandlerFunc(h.searchOutcome))
}

func (h *Handlers) searchRoutes(r *webserver.Router) {
	r.Handle("GET /api/v1/titles/{id}/releases", webserver.PermManageLibrary, http.HandlerFunc(h.titleReleases))
	r.Handle("POST /api/v1/releases/{id}/choose", webserver.PermManageLibrary, http.HandlerFunc(h.chooseRelease))
	r.Handle("POST /api/v1/releases/{id}/turn-down", webserver.PermManageLibrary, http.HandlerFunc(h.turnDownRelease))
	r.Handle("GET /api/v1/held", webserver.PermManageLibrary, h.listState("held"))
	r.Handle("DELETE /api/v1/held", webserver.PermManageLibrary, h.removeFrom("held", false))
	r.Handle("DELETE /api/v1/held/{id}", webserver.PermManageLibrary, h.removeFrom("held", true))
	r.Handle("GET /api/v1/turned-down", webserver.PermManageLibrary, h.listState("turned_down"))
	r.Handle("DELETE /api/v1/turned-down", webserver.PermManageLibrary, h.removeFrom("turned_down", false))
	r.Handle("DELETE /api/v1/turned-down/{id}", webserver.PermManageLibrary, h.removeFrom("turned_down", true))
	r.Handle("POST /api/v1/search/missing", webserver.PermManageLibrary, h.bulkSearch("missing"))
	r.Handle("POST /api/v1/search/better", webserver.PermManageLibrary, h.bulkSearch("better"))
	r.Handle("POST /api/v1/search/feed-check", webserver.PermManageLibrary, http.HandlerFunc(h.feedCheckNow))
	r.Handle("GET /api/v1/searches", webserver.PermManageLibrary, http.HandlerFunc(h.searches))
	r.Handle("GET /api/v1/settings/search", webserver.PermSettings, http.HandlerFunc(h.getSearchSettings))
	r.Handle("PUT /api/v1/settings/search", webserver.PermSettings, http.HandlerFunc(h.putSearchSettings))
}

func (h *Handlers) bridgeRoutes(r *webserver.Router) {
	r.Handle("GET /api/v1/bridges", webserver.PermManageLibrary, http.HandlerFunc(h.listBridges))
	r.Handle("POST /api/v1/bridges", webserver.PermSettings, http.HandlerFunc(h.addBridge))
	r.Handle("POST /api/v1/bridges/find", webserver.PermSettings, http.HandlerFunc(h.findBridges))
	r.Handle("POST /api/v1/bridges/test", webserver.PermSettings, http.HandlerFunc(h.testBridge))
	r.Handle("POST /api/v1/bridges/{id}/check", webserver.PermSettings, http.HandlerFunc(h.checkBridge))
	r.Handle("DELETE /api/v1/bridges/{id}", webserver.PermSettings, http.HandlerFunc(h.deleteBridge))
}

func (h *Handlers) search() *Searcher { return h.Search() }

// actorOf is the signed-in person; without sign-in (tests) it is the owner.
func actorOf(r *http.Request) domain.Actor {
	if _, ok := auth.From(r.Context()); ok {
		return auth.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

// pathID reads a positive {id} path value or answers 404.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// queryLimit reads limit (1 to 500) from the query, or def.
func queryLimit(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return def
	}
	return n
}

// queryPage reads limit (default def) and offset (0 up) from the query.
func queryPage(r *http.Request, def int) (int, int) {
	off, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || off < 0 {
		off = 0
	}
	return queryLimit(r, def), off
}
