package api

import (
	"errors"
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/health"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// healthRoutes are the health check routes (5.1.13).
func (a *API) healthRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/health/checks", a.healthChecks)
	m.HandleFunc("POST /api/v1/health/checks/run", a.runHealthChecks)
	m.HandleFunc("GET /api/v1/health/states", a.healthStates)
	m.HandleFunc("GET /api/v1/settings/health", a.getHealthSettings)
	m.HandleFunc("PUT /api/v1/settings/health", a.putHealthSettings)
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
