package api

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/source"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// searchOutcome tells whether a Title's newest search came back empty.
// Owners and admins (settings) also get the Sources that were paused or
// failed, so they can fix them; everyone else only sees that nothing was
// found yet.
func (a *API) searchOutcome(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	es, _, err := a.s.Store.Events(r.Context(), store.EventFilter{TitleID: t.ID, Limit: 1,
		Kinds: []string{source.EventSearchDone, source.EventSearchFailed, source.EventReleaseChosen}})
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := map[string]any{"empty": false}
	if len(es) == 0 || es[0].Kind == source.EventReleaseChosen {
		writeJSON(w, http.StatusOK, out)
		return
	}
	e := es[0]
	out["empty"] = true
	out["at"] = e.At
	who, signed := auth.From(r.Context())
	if signed && who.Has(auth.PermSettings) {
		srcs := []any{}
		if list, ok := e.Data["sources"].([]any); ok {
			srcs = list
		}
		out["sources"] = srcs
	}
	writeJSON(w, http.StatusOK, out)
}
