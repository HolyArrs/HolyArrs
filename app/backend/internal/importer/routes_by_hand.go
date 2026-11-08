package importer

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Deps is what the import-by-hand routes need.
type Deps struct {
	Store *database.Store
	// SavePath is the downloads folder imports may read from.
	SavePath string
	// Live gets the import events; nil for none.
	Live Publisher
}

// Handlers serve Import files by hand: scan a folder for
// videos with a guess of each, then import the chosen files.
type Handlers struct {
	Deps
}

// Routes registers the import-by-hand routes and the import folders setting.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/manual-import/scan", h.scanByHand)
	m.HandleFunc("POST /api/v1/manual-import", h.importByHand)
	m.HandleFunc("GET /api/v1/settings/import-folders", h.getImportFolders)
	m.HandleFunc("PUT /api/v1/settings/import-folders", h.putImportFolders)
}

func (h *Handlers) byHandImporter() *Importer {
	return &Importer{Store: h.Store, SavePath: h.SavePath, Live: h.Live}
}

func (h *Handlers) scanByHand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	const want = `send {"path": "<the full path of a folder or file>"}`
	if !webserver.Decode(w, r, &body, want) {
		return
	}
	out, err := h.byHandImporter().ScanByHand(r.Context(), body.Path)
	var outside *PlaceError
	if errors.As(err, &outside) || errors.Is(err, ErrByHandPath) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) importByHand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []ByHandItem `json:"items"`
	}
	const want = `send {"items": [{"path": "<file>", "slot_id": <id>, "mode": "copy"}]}, or "title_id" with "season" and "episode" for a show in place of "slot_id"`
	if !webserver.Decode(w, r, &body, want) {
		return
	}
	if len(body.Items) == 0 || len(body.Items) > 500 {
		webserver.Fail(w, http.StatusBadRequest, "send 1 to 500 files to import")
		return
	}
	res, err := h.byHandImporter().ImportByHand(r.Context(), body.Items, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
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
func (h *Handlers) getImportFolders(w http.ResponseWriter, r *http.Request) {
	ps, err := h.byHandImporter().ByHandPlaces(r.Context())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ps)
}

// putImportFolders saves the extra import folders: the owner or an admin,
// signed in with a password or an admin key, since they widen what import
// by hand may read.
func (h *Handlers) putImportFolders(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Extra []string `json:"extra_folders"`
	}
	if !webserver.Decode(w, r, &body, `send {"extra_folders": ["/data/imports"]}, or [] for none`) {
		return
	}
	if _, err := SaveImportFolders(r.Context(), h.Store, body.Extra, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1})); err != nil {
		if errors.Is(err, ErrImportFolder) {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		webserver.Internal(w, "api", err)
		return
	}
	h.getImportFolders(w, r)
}
