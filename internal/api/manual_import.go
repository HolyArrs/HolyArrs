package api

import (
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/importer"
)

// manualImportRoutes are Import files by hand (roadmap #33): scan a folder
// for videos with a guess of each, then import the chosen files.
func (a *API) manualImportRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/manual-import/scan", a.scanByHand)
	m.HandleFunc("POST /api/v1/manual-import", a.importByHand)
}

func (a *API) byHandImporter() *importer.Importer {
	im := &importer.Importer{Store: a.s.Store}
	if a.s.Live != nil {
		im.Live = a.s.Live // a nil hub must stay a nil Publisher
	}
	return im
}

func (a *API) scanByHand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	const want = `send {"path": "<the full path of a folder or file>"}`
	if !decode(w, r, &body, want) {
		return
	}
	root, err := importer.CleanByHandPath(body.Path)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := a.byHandImporter().ScanByHand(r.Context(), root)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) importByHand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []importer.ByHandItem `json:"items"`
	}
	const want = `send {"items": [{"path": "<file>", "slot_id": <id>, "mode": "copy"}]}, or "title_id" with "season" and "episode" for a show in place of "slot_id"`
	if !decode(w, r, &body, want) {
		return
	}
	if len(body.Items) == 0 || len(body.Items) > 500 {
		fail(w, http.StatusBadRequest, "send 1 to 500 files to import")
		return
	}
	res, err := a.byHandImporter().ImportByHand(r.Context(), body.Items, actor(r))
	if err != nil {
		internal(w, err)
		return
	}
	placed := 0
	for _, x := range res {
		if x.Outcome == "placed" {
			placed++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": res, "placed": placed})
}
