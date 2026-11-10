package library

import (
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	mf "github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// FolderHandlers serve the Library folder, Never-add and release-gate
// settings routes.
type FolderHandlers struct {
	Store         *database.Store
	DownloadPaths func(*http.Request) []string
	Actor         func(*http.Request) domain.Actor
	// Fail answers a library refusal with a status and a reason.
	Fail func(http.ResponseWriter, error)
}

// Routes registers the folder, Never-add and release-gate routes.
func (h *FolderHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/library-folders", h.listFolders)
	m.HandleFunc("POST /api/v1/library-folders", h.addFolder)
	m.HandleFunc("PUT /api/v1/library-folders/{id}", h.editFolder)
	m.HandleFunc("DELETE /api/v1/library-folders/{id}", h.deleteFolder)
	m.HandleFunc("POST /api/v1/library-folders/{id}/check", h.checkFolder)
	m.HandleFunc("GET /api/v1/never-add", h.listNeverAdd)
	m.HandleFunc("POST /api/v1/never-add", h.addNeverAdd)
	m.HandleFunc("DELETE /api/v1/never-add/{id}", h.deleteNeverAdd)
	m.HandleFunc("GET /api/v1/settings/release-gate", h.getGateDefaults)
	m.HandleFunc("PUT /api/v1/settings/release-gate", h.putGateDefaults)
}

// FolderView is a Library folder as the API shows it.
type FolderView struct {
	ID               int64      `json:"id"`
	Path             string     `json:"path"`
	Kind             string     `json:"kind"`
	Label            string     `json:"label"`
	DefaultPickRules *int64     `json:"default_pick_rules_id"`
	FreeSpace        int64      `json:"free_space"`
	Health           string     `json:"health"`
	HealthReason     string     `json:"health_reason"`
	HealthCheckedAt  *time.Time `json:"health_checked_at,omitempty"`
	CopiesExpected   bool       `json:"copies_expected"`
}

func folderView(f mf.LibraryFolder) FolderView {
	return FolderView{ID: f.ID, Path: f.Path, Kind: f.Kind, Label: f.Label, DefaultPickRules: f.DefaultPickRules, FreeSpace: f.FreeSpace,
		Health: f.Health, HealthReason: f.HealthReason, HealthCheckedAt: f.HealthCheckedAt, CopiesExpected: f.CopiesExpected}
}

// NeverAddView is a Never-add entry as the API shows it.
type NeverAddView struct {
	ID        int64             `json:"id"`
	Kind      string            `json:"kind"`
	SourceIDs map[string]string `json:"ids"`
	Name      string            `json:"name"`
	Reason    string            `json:"reason"`
	CreatedAt time.Time         `json:"created_at"`
}

func neverAddView(n mf.NeverAdd) NeverAddView {
	return NeverAddView{ID: n.ID, Kind: n.Kind, SourceIDs: n.SourceIDs, Name: n.Name, Reason: n.Reason, CreatedAt: n.CreatedAt}
}

func (h *FolderHandlers) listFolders(w http.ResponseWriter, r *http.Request) {
	fs, err := database.Select[mf.LibraryFolder](r.Context(), h.Store.Read(), "library_folders", "1=1 ORDER BY id")
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []FolderView{}
	for _, f := range fs {
		out = append(out, folderView(f))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"library_folders": out})
}

func (h *FolderHandlers) addFolder(w http.ResponseWriter, r *http.Request) {
	var in FolderInput
	if !webserver.Decode(w, r, &in, `send {"path": "/movies", "kind": "movie", "label": "Movies"}`) {
		return
	}
	lf, err := AddFolder(r.Context(), h.Store, in, h.DownloadPaths(r), h.Actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, folderView(lf))
}

func (h *FolderHandlers) editFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var c FolderChange
	if !webserver.Decode(w, r, &c, `send {"label": "...", "default_pick_rules_id": <id>, "copies_expected": true}`) {
		return
	}
	lf, err := UpdateFolder(r.Context(), h.Store, id, c, h.Actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, folderView(lf))
}

func (h *FolderHandlers) deleteFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := DeleteFolder(r.Context(), h.Store, id); err != nil {
		h.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FolderHandlers) checkFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lf, _, err := RecheckFolder(r.Context(), h.Store, id, h.DownloadPaths(r), h.Actor(r), false)
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, folderView(lf))
}

func (h *FolderHandlers) listNeverAdd(w http.ResponseWriter, r *http.Request) {
	ns, err := NeverAddList(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []NeverAddView{}
	for _, n := range ns {
		out = append(out, neverAddView(n))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"never_add": out})
}

func (h *FolderHandlers) addNeverAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind   string `json:"kind"`
		TMDB   string `json:"tmdb_id"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	if !webserver.Decode(w, r, &body, `send {"tmdb_id": "<number>", "name": "...", "reason": "...", "kind": "movie" or "show"}`) {
		return
	}
	if body.TMDB == "" || body.Name == "" || (body.Kind != "" && body.Kind != "movie" && body.Kind != "show") {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id and name are needed; kind is movie (the default) or show")
		return
	}
	n, err := AddNeverAdd(r.Context(), h.Store, mf.NeverAdd{Kind: body.Kind, SourceIDs: map[string]string{"tmdb": body.TMDB}, Name: body.Name, Reason: body.Reason}, h.Actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, neverAddView(n))
}

func (h *FolderHandlers) deleteNeverAdd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := RemoveNeverAdd(r.Context(), h.Store, id); err != nil {
		h.Fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FolderHandlers) getGateDefaults(w http.ResponseWriter, r *http.Request) {
	g, err := ReadGateDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, g)
}

func (h *FolderHandlers) putGateDefaults(w http.ResponseWriter, r *http.Request) {
	var g GateDefaults
	if !webserver.Decode(w, r, &g, `send {"release_gate": "released", "gate_delay_days": 0, "bulk_search_respects_gate": true}`) {
		return
	}
	if err := SetGateDefaults(r.Context(), h.Store, g, h.Actor(r)); err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, g)
}
