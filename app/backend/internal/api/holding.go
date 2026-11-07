package api

import (
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/library"
)

// holdingRoutes list the deleted files still in the holding folders and
// put them back.
func (a *API) holdingRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/holding", a.listHolding)
	m.HandleFunc("POST /api/v1/holding/restore", a.restoreHolding)
}

func (a *API) listHolding(w http.ResponseWriter, r *http.Request) {
	files, err := a.s.Library.HeldFiles(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// restoreHolding moves held files back; a deleted title comes back with
// its file and a rescan is queued so the Library sees it again.
func (a *API) restoreHolding(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &body, `send {"ids": [<file id>, ...]}`) {
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 5000 {
		fail(w, http.StatusBadRequest, "send 1 to 5000 ids")
		return
	}
	res, back, err := a.s.Library.RestoreHeld(r.Context(), body.IDs, actor(r))
	if err != nil {
		internal(w, err)
		return
	}
	out := map[string]any{"results": res, "titles_back": back}
	if len(back) > 0 {
		if id, _, err := a.s.Queue.Enqueue(r.Context(), library.KindRescan, nil, nil, library.KindRescan, actor(r)); err == nil {
			out["rescan_job_id"] = id
		}
	}
	writeJSON(w, http.StatusOK, out)
}
