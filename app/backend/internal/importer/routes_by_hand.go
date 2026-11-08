package importer

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// manualImportRoutes are Import files by hand (roadmap #33): scan a folder
// for videos with a guess of each, then import the chosen files.
func (a *API) manualImportRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/manual-import/scan", a.scanByHand)
	m.HandleFunc("POST /api/v1/manual-import", a.importByHand)
	m.HandleFunc("GET /api/v1/settings/import-folders", a.getImportFolders)
	m.HandleFunc("PUT /api/v1/settings/import-folders", a.putImportFolders)
}

func (a *API) byHandImporter() *importer.Importer {
	im := &importer.Importer{Store: a.s.Store, SavePath: a.s.SavePath}
	if im.SavePath == "" && a.s.Library != nil {
		im.SavePath = a.s.Library.SavePath
	}
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
	out, err := a.byHandImporter().ScanByHand(r.Context(), body.Path)
	var outside *importer.PlaceError
	if errors.As(err, &outside) || errors.Is(err, importer.ErrByHandPath) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
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
		webserver.Fail(w, http.StatusBadRequest, "send 1 to 500 files to import")
		return
	}
	res, err := a.byHandImporter().ImportByHand(r.Context(), body.Items, actor(r))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	placed := 0
	for _, x := range res {
		if x.Outcome == "placed" {
			placed++
		}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"results": res, "placed": placed})
}

// getImportFolders answers the places files may be imported by hand from:
// the Library folders, the Downloaders' folders and the extra ones.
func (a *API) getImportFolders(w http.ResponseWriter, r *http.Request) {
	ps, err := a.byHandImporter().ByHandPlaces(r.Context())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ps)
}

// putImportFolders saves the extra import folders: the owner or an admin,
// signed in with a password or an admin key, since they widen what import
// by hand may read.
func (a *API) putImportFolders(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Extra []string `json:"extra_folders"`
	}
	if !decode(w, r, &body, `send {"extra_folders": ["/data/imports"]}, or [] for none`) {
		return
	}
	if _, err := importer.SaveImportFolders(r.Context(), a.s.Store, body.Extra, actor(r)); err != nil {
		if errors.Is(err, importer.ErrImportFolder) {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		webserver.Internal(w, "api", err)
		return
	}
	a.getImportFolders(w, r)
}
