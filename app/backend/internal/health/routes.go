package health

import (
	"context"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// KeyKitForOthers is the key kit warning for people who are not the owner:
// only the owner can download the kit.
const KeyKitForOthers = "Ask the owner for the key kit: Backups don't include the master key"

// Handlers serves the health check routes and the health settings.
type Handlers struct {
	Store  *database.Store
	Engine *Engine
	// States is the supervisor's stage registry (may be nil).
	States *jobs.Registry
	// SafeMode says the app started with only the core.
	SafeMode bool
	// PartStatuses lists the parts' and stages' switches (nil: none).
	PartStatuses func() []jobs.Status
	// SyncTrouble lists the torrent clients whose background reads fail;
	// downloads owns it.
	SyncTrouble func(ctx context.Context) any
	// DefaultFeed is the release feed used when the setting is empty.
	DefaultFeed string
	// ResetUpdate forgets the update check's last answer after the
	// settings change, so the next check asks again.
	ResetUpdate func(ctx context.Context)
}

// DiskView is one library folder's disk: free and total bytes (-1 unknown).
type DiskView struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Free  int64  `json:"free_bytes"`
	Total int64  `json:"total_bytes"`
}

// Routes adds the health check routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/health/checks", h.checks)
	m.HandleFunc("POST /api/v1/health/checks/run", h.runChecks)
	m.HandleFunc("GET /api/v1/health/states", h.states)
	m.HandleFunc("GET /api/v1/health/disks", h.disks)
	m.HandleFunc("GET /api/v1/settings/health", h.getSettings)
	m.HandleFunc("PUT /api/v1/settings/health", h.putSettings)
}

// NotOwner says whether the caller is signed in and is not the owner.
func NotOwner(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	return ok && who.Role != webserver.RoleOwner
}

func (h *Handlers) checks(w http.ResponseWriter, r *http.Request) {
	cs, err := List(r.Context(), h.Store)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.answerChecks(w, r, cs)
}

func (h *Handlers) runChecks(w http.ResponseWriter, r *http.Request) {
	cs, err := h.Engine.Run(r.Context())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.answerChecks(w, r, cs)
}

// answerChecks sends the checks, the worst state first, and the clients
// whose background reads fail.
func (h *Handlers) answerChecks(w http.ResponseWriter, r *http.Request, cs []domain.HealthCheck) {
	worst := OK
	if len(cs) > 0 {
		worst = cs[0].State
	}
	ownerWords(r, cs)
	out := map[string]any{"checks": cs, "worst": worst}
	out["sync"] = h.SyncTrouble(r.Context())
	webserver.WriteJSON(w, http.StatusOK, out)
}

// states answers the state of every stage and every linked app, and
// whether the app runs in safe mode.
func (h *Handlers) states(w http.ResponseWriter, r *http.Request) {
	var states []jobs.Status
	if h.States != nil {
		states = h.States.List()
	}
	links, err := h.Engine.Links(r.Context())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if h.PartStatuses != nil {
		states = append(states, h.PartStatuses()...)
	}
	states = append(states, links...)
	if states == nil {
		states = []jobs.Status{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"safe_mode": h.SafeMode, "states": states})
}

func (h *Handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	s, err := ReadSettings(r.Context(), h.Store, h.DefaultFeed)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, s)
}

func (h *Handlers) putSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if !webserver.Decode(w, r, &in, `send {"every_minutes": 5, "flap_guard": 2, "low_space_mb": 1024, "update_check_url": ""}`) {
		return
	}
	if err := SaveSettings(r.Context(), h.Store, in, webserver.RequestActor(r)); err != nil {
		if errors.Is(err, ErrSettings) {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		webserver.Internal(w, "api", err)
		return
	}
	h.ResetUpdate(r.Context())
	h.getSettings(w, r)
}

// disks answers free and total space for every library folder, for the
// storage bars on the Health page.
func (h *Handlers) disks(w http.ResponseWriter, r *http.Request) {
	ds, err := libraryFolders(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []DiskView{}
	for _, d := range ds {
		d.Free, d.Total = importer.FreeSpace(d.Path), importer.DiskSize(d.Path)
		out = append(out, d)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"disks": out})
}

// ownerWords rewords the key kit warning for people who are not the owner.
func ownerWords(r *http.Request, cs []domain.HealthCheck) {
	if who, ok := webserver.From(r.Context()); !ok || who.Role == webserver.RoleOwner {
		return
	}
	for i := range cs {
		if cs[i].Key == "backups:key" && cs[i].State != OK {
			cs[i].Message = KeyKitForOthers
		}
	}
}
