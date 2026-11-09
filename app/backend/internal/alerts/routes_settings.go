package alerts

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// getSettings answers the Alert settings that belong to no single Alert,
// such as how a pack import is told.
func (h *Handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, ReadSettings(r.Context(), h.Store.Read()))
}

func (h *Handlers) putSettings(w http.ResponseWriter, r *http.Request) {
	st := ReadSettings(r.Context(), h.Store.Read()) // fields left out keep their value
	if !webserver.Decode(w, r, &st, `send {"pack_alerts": "one"} for one Alert per pack import, or "each" for one per episode (each_limit: at most that many, 1 to 500, then one summary)`) {
		return
	}
	if err := SaveSettings(r.Context(), h.Store, st, webserver.RequestActor(r)); err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	webserver.WriteJSON(w, http.StatusOK, st)
}
