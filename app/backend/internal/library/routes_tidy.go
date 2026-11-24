package library

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// previewTidy answers the Tidy my folders preview: where each loose movie
// would go, the ones that cannot move and why, and the ones with no year.
func (h *FolderHandlers) previewTidy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := PlanTidy(r.Context(), h.Store.Read(), id)
	if err != nil {
		h.tidyFail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, p)
}

// applyTidy moves the files of the preview the hash names.
func (h *FolderHandlers) applyTidy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Hash string `json:"hash"`
	}
	if !webserver.Decode(w, r, &body, `send {"hash": "<the preview's hash>"}`) {
		return
	}
	res, err := ApplyTidy(r.Context(), h.Store, id, body.Hash, h.Actor(r))
	if err != nil {
		h.tidyFail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, res)
}

// undoTidy puts the files of the last tidy back.
func (h *FolderHandlers) undoTidy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := UndoTidy(r.Context(), h.Store, id, h.Actor(r))
	if err != nil {
		h.tidyFail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, res)
}

func (h *FolderHandlers) tidyFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoFolder):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrTidyChanged), errors.Is(err, ErrNothingToUndo), errors.Is(err, mediafiles.ErrLookOnly):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotMovieFolder):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	default:
		h.Fail(w, err)
	}
}
