package library

import (
	"context"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// VersionHandlers serves adding, changing and removing a Title's Versions.
type VersionHandlers struct {
	Store *database.Store
	Lib   *Service
	// TitleID reads the {id} Title, answering 404 itself.
	TitleID func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// Publish sends the changed Title to live pages.
	Publish func(r *http.Request, id int64)
	// View is how a Version is shown.
	View func(ctx context.Context, db database.Querier, v mediafiles.Version) any
	// Fail answers a library error.
	Fail func(w http.ResponseWriter, err error)
	// DownloadPaths are the downloaders' save paths, which a Version folder may not use.
	DownloadPaths func(r *http.Request) []string
}

// Routes registers POST, PUT and DELETE /api/v1/titles/{id}/versions.
func (h *VersionHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/titles/{id}/versions", h.addVersion)
	m.HandleFunc("PUT /api/v1/titles/{id}/versions/{vid}", h.editVersion)
	m.HandleFunc("DELETE /api/v1/titles/{id}/versions/{vid}", h.deleteVersion)
}

// actor is the signed-in person, or the owner when no session is attached.
func actor(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

func versionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("vid"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

func (h *VersionHandlers) addVersion(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	var in VersionInput
	if !webserver.Decode(w, r, &in, `send {"label": "4K"} with optional pick_rules_id, library_folder_id, keep_up_to_date`) {
		return
	}
	v, err := AddVersion(r.Context(), h.Store, t.ID, in, h.DownloadPaths(r), h.Lib.Clock(), actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	h.Lib.VersionEvent(r.Context(), t, "Added the "+v.Label+" Version", v, actor(r))
	h.Publish(r, t.ID)
	webserver.WriteJSON(w, http.StatusCreated, h.View(r.Context(), h.Store.Read(), v))
}

func (h *VersionHandlers) editVersion(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	vid, ok := versionID(w, r)
	if !ok {
		return
	}
	var c VersionChange
	if !webserver.Decode(w, r, &c, `send any of label, pick_rules_id, keep_up_to_date`) {
		return
	}
	v, err := EditVersion(r.Context(), h.Store, t.ID, vid, c, actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	h.Lib.VersionEvent(r.Context(), t, "Changed the "+v.Label+" Version", v, actor(r))
	h.Publish(r, t.ID)
	webserver.WriteJSON(w, http.StatusOK, h.View(r.Context(), h.Store.Read(), v))
}

func (h *VersionHandlers) deleteVersion(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	vid, ok := versionID(w, r)
	if !ok {
		return
	}
	var v mediafiles.Version
	_ = database.Get(r.Context(), h.Store.Read(), "versions", vid, &v)
	if err := RemoveVersion(r.Context(), h.Store, t.ID, vid, actor(r)); err != nil {
		h.Fail(w, err)
		return
	}
	h.Lib.VersionEvent(r.Context(), t, "Removed the "+v.Label+" Version", v, actor(r))
	h.Publish(r, t.ID)
	w.WriteHeader(http.StatusNoContent)
}
