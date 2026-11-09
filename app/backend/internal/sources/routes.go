package sources

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

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

	bridgeOnce  sync.Once
	bridgeSvc   *BridgeService
	bridgeLimit bridgeLimiter
}

// Routes registers every route of this module.
func (h *Handlers) Routes(m *webserver.Router) {
	h.sourceRoutes(m)
	h.searchRoutes(m)
	h.bridgeRoutes(m)
}

func (h *Handlers) sourceRoutes(m *webserver.Router) {
	m.Handle("GET /api/v1/sources", webserver.PermManageLibrary, http.HandlerFunc(h.listSources))
	m.Handle("POST /api/v1/sources", webserver.PermSettings, http.HandlerFunc(h.addSource))
	m.Handle("PUT /api/v1/sources/{id}", webserver.PermSettings, http.HandlerFunc(h.editSource))
	m.Handle("DELETE /api/v1/sources/{id}", webserver.PermSettings, http.HandlerFunc(h.deleteSource))
	m.Handle("POST /api/v1/sources/{id}/test", webserver.PermSettings, http.HandlerFunc(h.TestSource))
	m.Handle("POST /api/v1/sources/{id}/pause", webserver.PermSettings, http.HandlerFunc(h.pauseSource))
	m.Handle("POST /api/v1/sources/{id}/resume", webserver.PermSettings, http.HandlerFunc(h.resumeSource))
	m.Handle("GET /api/v1/sources/{id}/stats", webserver.PermManageLibrary, http.HandlerFunc(h.sourceStats))
	m.Handle("POST /api/v1/sources/{id}/try", webserver.PermSettings, http.HandlerFunc(h.trySource))
	m.Handle("POST /api/v1/sources/check", webserver.PermSettings, http.HandlerFunc(h.checkSources))
	m.Handle("GET /api/v1/titles/{id}/search-outcome", webserver.NeedSignedIn, http.HandlerFunc(h.searchOutcome))
}

func (h *Handlers) searchRoutes(m *webserver.Router) {
	m.Handle("GET /api/v1/titles/{id}/releases", webserver.PermManageLibrary, http.HandlerFunc(h.titleReleases))
	m.Handle("POST /api/v1/releases/{id}/choose", webserver.PermManageLibrary, http.HandlerFunc(h.chooseRelease))
	m.Handle("POST /api/v1/releases/{id}/turn-down", webserver.PermManageLibrary, http.HandlerFunc(h.turnDownRelease))
	m.Handle("GET /api/v1/held", webserver.PermManageLibrary, h.listState("held"))
	m.Handle("DELETE /api/v1/held", webserver.PermManageLibrary, h.removeFrom("held", false))
	m.Handle("DELETE /api/v1/held/{id}", webserver.PermManageLibrary, h.removeFrom("held", true))
	m.Handle("GET /api/v1/turned-down", webserver.PermManageLibrary, h.listState("turned_down"))
	m.Handle("DELETE /api/v1/turned-down", webserver.PermManageLibrary, h.removeFrom("turned_down", false))
	m.Handle("DELETE /api/v1/turned-down/{id}", webserver.PermManageLibrary, h.removeFrom("turned_down", true))
	m.Handle("POST /api/v1/search/missing", webserver.PermManageLibrary, h.bulkSearch("missing"))
	m.Handle("POST /api/v1/search/better", webserver.PermManageLibrary, h.bulkSearch("better"))
	m.Handle("POST /api/v1/search/feed-check", webserver.PermManageLibrary, http.HandlerFunc(h.feedCheckNow))
	m.Handle("GET /api/v1/searches", webserver.PermManageLibrary, http.HandlerFunc(h.searches))
	m.Handle("GET /api/v1/settings/search", webserver.PermSettings, http.HandlerFunc(h.getSearchSettings))
	m.Handle("PUT /api/v1/settings/search", webserver.PermSettings, http.HandlerFunc(h.putSearchSettings))
}

func (h *Handlers) bridgeRoutes(m *webserver.Router) {
	m.Handle("GET /api/v1/bridges", webserver.PermManageLibrary, http.HandlerFunc(h.listBridges))
	m.Handle("POST /api/v1/bridges", webserver.PermSettings, http.HandlerFunc(h.addBridge))
	m.Handle("POST /api/v1/bridges/find", webserver.PermSettings, http.HandlerFunc(h.findBridges))
	m.Handle("POST /api/v1/bridges/test", webserver.PermSettings, http.HandlerFunc(h.testBridge))
	m.Handle("POST /api/v1/bridges/{id}/check", webserver.PermSettings, http.HandlerFunc(h.checkBridge))
	m.Handle("DELETE /api/v1/bridges/{id}", webserver.PermSettings, http.HandlerFunc(h.deleteBridge))
}

func (h *Handlers) search() *Searcher { return h.Search() }

// actorOf is the signed-in person; without sign-in (tests) it is the owner.
func actorOf(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
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
