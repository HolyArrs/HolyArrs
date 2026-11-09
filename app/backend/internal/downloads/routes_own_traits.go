package downloads

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// OwnTraitView is one of your own traits. Key is how Pick rules name it.
type OwnTraitView struct {
	ID    int64    `json:"id"`
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Match string   `json:"match"`
	Terms []string `json:"terms"`
}

func viewOfOwnTrait(t mediafiles.OwnTrait) OwnTraitView {
	terms := t.Terms
	if terms == nil {
		terms = []string{}
	}
	return OwnTraitView{ID: t.ID, Key: mediafiles.OwnTraitKey(t.ID), Name: t.Name, Match: t.Match, Terms: terms}
}

const ownTraitHelp = `send {"name": "...", "match": "name" or "group", "terms": ["word", "/pattern/"]}`

func (h *Handlers) ownTraitID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "the trait id must be a number")
		return 0, false
	}
	return id, true
}

func ownTraitError(w http.ResponseWriter, err error) {
	var fe *mediafiles.FieldError
	switch {
	case errors.As(err, &fe):
		webserver.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": fe.Text, "field": fe.Field})
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such trait")
	case errors.Is(err, library.ErrOwnTraitName):
		webserver.WriteJSON(w, http.StatusConflict, map[string]string{"error": "Another of your traits already has this name.", "field": "name"})
	default:
		webserver.Internal(w, "downloads", err)
	}
}

func (h *Handlers) listOwnTraits(w http.ResponseWriter, r *http.Request) {
	ts, err := library.OwnTraits(r.Context(), h.Store)
	if err != nil {
		webserver.Internal(w, "downloads", err)
		return
	}
	out := []OwnTraitView{}
	for _, t := range ts {
		out = append(out, viewOfOwnTrait(t))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"own_traits": out})
}

func (h *Handlers) addOwnTrait(w http.ResponseWriter, r *http.Request) {
	var in library.OwnTraitInput
	if !webserver.Decode(w, r, &in, ownTraitHelp) {
		return
	}
	t, err := library.CreateOwnTrait(r.Context(), h.Store, in, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		ownTraitError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, viewOfOwnTrait(t))
}

func (h *Handlers) editOwnTrait(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ownTraitID(w, r)
	if !ok {
		return
	}
	var in library.OwnTraitInput
	if !webserver.Decode(w, r, &in, ownTraitHelp) {
		return
	}
	t, err := library.UpdateOwnTrait(r.Context(), h.Store, id, in, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		ownTraitError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, viewOfOwnTrait(t))
}

// deleteOwnTrait answers how many Pick rules lost the trait.
func (h *Handlers) deleteOwnTrait(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ownTraitID(w, r)
	if !ok {
		return
	}
	n, err := library.DeleteOwnTrait(r.Context(), h.Store, id, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		ownTraitError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]int{"pick_rules_changed": n})
}
