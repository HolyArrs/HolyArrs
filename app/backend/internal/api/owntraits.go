package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// OwnTraitView is one of your own traits (#97). Key is how Pick rules name it.
type OwnTraitView struct {
	ID    int64    `json:"id"`
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Match string   `json:"match"`
	Terms []string `json:"terms"`
}

func viewOfOwnTrait(t domain.OwnTrait) OwnTraitView {
	terms := t.Terms
	if terms == nil {
		terms = []string{}
	}
	return OwnTraitView{ID: t.ID, Key: domain.OwnTraitKey(t.ID), Name: t.Name, Match: t.Match, Terms: terms}
}

const ownTraitHelp = `send {"name": "...", "match": "name" or "group", "terms": ["word", "/pattern/"]}`

func (a *API) ownTraitID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "the trait id must be a number")
		return 0, false
	}
	return id, true
}

func ownTraitError(w http.ResponseWriter, err error) {
	var fe *domain.FieldError
	switch {
	case errors.As(err, &fe):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fe.Text, "field": fe.Field})
	case errors.Is(err, database.ErrNotFound):
		fail(w, http.StatusNotFound, "no such trait")
	case errors.Is(err, library.ErrOwnTraitName):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Another of your traits already has this name.", "field": "name"})
	default:
		internal(w, err)
	}
}

func (a *API) listOwnTraits(w http.ResponseWriter, r *http.Request) {
	ts, err := library.OwnTraits(r.Context(), a.s.Store)
	if err != nil {
		internal(w, err)
		return
	}
	out := []OwnTraitView{}
	for _, t := range ts {
		out = append(out, viewOfOwnTrait(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"own_traits": out})
}

func (a *API) addOwnTrait(w http.ResponseWriter, r *http.Request) {
	var in library.OwnTraitInput
	if !decode(w, r, &in, ownTraitHelp) {
		return
	}
	t, err := library.CreateOwnTrait(r.Context(), a.s.Store, in, actor(r))
	if err != nil {
		ownTraitError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, viewOfOwnTrait(t))
}

func (a *API) editOwnTrait(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ownTraitID(w, r)
	if !ok {
		return
	}
	var in library.OwnTraitInput
	if !decode(w, r, &in, ownTraitHelp) {
		return
	}
	t, err := library.UpdateOwnTrait(r.Context(), a.s.Store, id, in, actor(r))
	if err != nil {
		ownTraitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOfOwnTrait(t))
}

// deleteOwnTrait answers how many Pick rules lost the trait.
func (a *API) deleteOwnTrait(w http.ResponseWriter, r *http.Request) {
	id, ok := a.ownTraitID(w, r)
	if !ok {
		return
	}
	n, err := library.DeleteOwnTrait(r.Context(), a.s.Store, id, actor(r))
	if err != nil {
		ownTraitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"pick_rules_changed": n})
}
