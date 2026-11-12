package mediafiles

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// libraryStats answers the Library's files by fact for the kinds whose part is on.
func (h *Handlers) libraryStats(w http.ResponseWriter, r *http.Request) {
	st, err := LibraryStatsOf(r.Context(), h.Store, h.KindShown)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, st)
}
