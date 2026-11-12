package mediaservers

import (
	"encoding/json"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// watchSetting is the owner's "Read watch history" switch.
type watchSetting struct {
	Enabled *bool `json:"enabled"`
}

func (h *Handlers) getWatchHistory(w http.ResponseWriter, r *http.Request) {
	on := WatchHistoryOn(r.Context(), h.Store.Read())
	webserver.WriteJSON(w, http.StatusOK, watchSetting{Enabled: &on})
}

// putWatchHistory turns watch history on or off; off deletes what was
// read, on starts a read right away.
func (h *Handlers) putWatchHistory(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var in watchSetting
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil || in.Enabled == nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"enabled": true} or {"enabled": false}`)
		return
	}
	if err := SetWatchHistory(r.Context(), h.Store, *in.Enabled, actorOf(r)); err != nil {
		internalError(w, err)
		return
	}
	if *in.Enabled {
		h.Linker.ReadWatchHistorySoon()
	}
	webserver.WriteJSON(w, http.StatusOK, in)
}

// watched answers who watched what, for people who manage the Library.
func (h *Handlers) watched(w http.ResponseWriter, r *http.Request) {
	sum, err := Watched(r.Context(), h.Store.Read())
	if err != nil {
		internalError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, sum)
}
