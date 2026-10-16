package api

import (
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/alerts"
)

// getAlertSettings answers the Alert settings that are not one Alert's own
// (5.2.5: how a pack import is told).
func (a *API) getAlertSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, alerts.ReadSettings(r.Context(), a.s.Store.Read()))
}

func (a *API) putAlertSettings(w http.ResponseWriter, r *http.Request) {
	st := alerts.ReadSettings(r.Context(), a.s.Store.Read()) // fields left out keep their value
	if !decode(w, r, &st, `send {"pack_alerts": "one"} for one Alert per pack import, or "each" for one per episode (each_limit: at most that many, 1 to 500, then one summary)`) {
		return
	}
	if err := alerts.SaveSettings(r.Context(), a.s.Store, st, actor(r)); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}
