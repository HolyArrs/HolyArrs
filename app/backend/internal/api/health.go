package api

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/supervisor"
)

// healthRoutes are the health check routes (5.1.13).
func (a *API) healthRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/health/checks", a.healthChecks)
	m.HandleFunc("POST /api/v1/health/checks/run", a.runHealthChecks)
	m.HandleFunc("GET /api/v1/health/states", a.healthStates)
	m.HandleFunc("GET /api/v1/health/disks", a.healthDisks)
	m.HandleFunc("GET /api/v1/settings/health", a.getHealthSettings)
	m.HandleFunc("PUT /api/v1/settings/health", a.putHealthSettings)
}

// keyKitForOthers is the key kit warning for people who are not the owner.
const keyKitForOthers = "Ask the owner for the key kit: Backups don't include the master key"

// notOwner says whether the caller is signed in and is not the owner.
func notOwner(r *http.Request) bool {
	who, ok := auth.From(r.Context())
	return ok && who.Role != auth.RoleOwner
}

// ownerWords rewords the key kit warning for people who are not the owner:
// only the owner can download the kit.
func ownerWords(r *http.Request, cs []domain.HealthCheck) {
	if who, ok := auth.From(r.Context()); !ok || who.Role == auth.RoleOwner {
		return
	}
	for i := range cs {
		if cs[i].Key == "backups:key" && cs[i].State != health.OK {
			cs[i].Message = keyKitForOthers
		}
	}
}

func healthSummary(cs any, worst string) map[string]any {
	return map[string]any{"checks": cs, "worst": worst}
}

func (a *API) healthChecks(w http.ResponseWriter, r *http.Request) {
	cs, err := health.List(r.Context(), a.s.Store)
	if err != nil {
		internal(w, err)
		return
	}
	worst := health.OK
	if len(cs) > 0 {
		worst = cs[0].State
	}
	ownerWords(r, cs)
	out := healthSummary(cs, worst)
	out["sync"] = a.syncTrouble(r.Context())
	writeJSON(w, http.StatusOK, out)
}

func (a *API) runHealthChecks(w http.ResponseWriter, r *http.Request) {
	cs, err := a.s.Health.Run(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	worst := health.OK
	if len(cs) > 0 {
		worst = cs[0].State
	}
	ownerWords(r, cs)
	out := healthSummary(cs, worst)
	out["sync"] = a.syncTrouble(r.Context())
	writeJSON(w, http.StatusOK, out)
}

// healthStates answers the state of every stage (from the supervisor's
// Registry) and every linked app, and whether the app runs in safe mode.
func (a *API) healthStates(w http.ResponseWriter, r *http.Request) {
	var states []supervisor.Status
	if a.s.States != nil {
		states = a.s.States.List()
	}
	links, err := a.s.Health.Links(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	if a.s.Parts != nil {
		states = append(states, a.s.Parts.Statuses()...)
	}
	states = append(states, links...)
	if states == nil {
		states = []supervisor.Status{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"safe_mode": a.s.SafeMode, "states": states})
}

func (a *API) getHealthSettings(w http.ResponseWriter, r *http.Request) {
	s, err := health.ReadSettings(r.Context(), a.s.Store)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *API) putHealthSettings(w http.ResponseWriter, r *http.Request) {
	var in health.Settings
	if !decode(w, r, &in, `send {"every_minutes": 5, "flap_guard": 2, "low_space_mb": 1024, "update_check_url": ""}`) {
		return
	}
	if err := health.SaveSettings(r.Context(), a.s.Store, in, actor(r)); err != nil {
		if errors.Is(err, health.ErrSettings) {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		internal(w, err)
		return
	}
	a.s.Health.ResetUpdate(r.Context())
	a.getHealthSettings(w, r)
}

// DiskView is one library folder's disk: free and total bytes (-1 unknown).
type DiskView struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Free  int64  `json:"free_bytes"`
	Total int64  `json:"total_bytes"`
}

// healthDisks answers free and total space for every library folder, for
// the Status page's storage bars.
func (a *API) healthDisks(w http.ResponseWriter, r *http.Request) {
	rows, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT path, kind, label FROM library_folders ORDER BY kind, path")
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []DiskView{}
	for rows.Next() {
		var d DiskView
		if err := rows.Scan(&d.Path, &d.Kind, &d.Label); err != nil {
			internal(w, err)
			return
		}
		d.Free, d.Total = importer.FreeSpace(d.Path), importer.DiskSize(d.Path)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"disks": out})
}
