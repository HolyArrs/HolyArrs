package sources

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// searchOutcome tells whether a Title's newest search came back empty.
// Owners and admins (settings) also get the Sources that were paused or
// failed, so they can fix them; everyone else only sees that nothing was
// found yet.
func (h *Handlers) searchOutcome(w http.ResponseWriter, r *http.Request) {
	t, ok := h.Catalog.TitleFromPath(w, r)
	if !ok {
		return
	}
	es, _, err := h.Store.Events(r.Context(), database.EventFilter{TitleID: t.ID, Limit: 1,
		Kinds: []string{EventSearchDone, EventSearchFailed, EventReleaseChosen}})
	if err != nil {
		webserver.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := map[string]any{"empty": false}
	if len(es) == 0 || es[0].Kind == EventReleaseChosen {
		webserver.WriteJSON(w, http.StatusOK, out)
		return
	}
	e := es[0]
	out["empty"] = true
	out["at"] = e.At
	who, signed := webserver.From(r.Context())
	if signed && who.Has(webserver.PermSettings) {
		srcs := []any{}
		if list, ok := e.Data["sources"].([]any); ok {
			srcs = list
		}
		out["sources"] = srcs
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}
