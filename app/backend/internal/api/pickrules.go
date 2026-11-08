package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// PickRulesDetail is one Pick rule with its body and what uses it.
type PickRulesDetail struct {
	ID       int64                    `json:"id"`
	Name     string                   `json:"name"`
	Kinds    []string                 `json:"kinds"`
	Preset   bool                     `json:"preset"`
	Revision int                      `json:"revision"`
	Body     mediafiles.PickRulesBody `json:"body"`
	InUse    library.Use              `json:"in_use"`
}

func detailOfPickRules(p mediafiles.PickRules, u library.Use) PickRulesDetail {
	k := p.Kinds
	if k == nil {
		k = []string{}
	}
	return PickRulesDetail{ID: p.ID, Name: p.Name, Kinds: k, Preset: p.Preset, Revision: p.Revision, Body: p.Body, InUse: u}
}

// useWords says what uses a Pick rule, for a 409.
func useWords(u library.Use) string {
	var parts []string
	add := func(n int, one, many string) {
		switch {
		case n == 1:
			parts = append(parts, "1 "+one)
		case n > 1:
			parts = append(parts, fmt.Sprintf("%d %s", n, many))
		}
	}
	add(u.Versions, "Version", "Versions")
	add(u.LibraryFolders, "Library folder", "Library folders")
	add(u.OutsideLists, "Outside list", "Outside lists")
	add(u.Wishlist, "Request setting", "Request settings")
	add(u.TitleDefaults, "Movies and TV default", "Movies and TV defaults")
	switch len(parts) {
	case 0:
		return "nothing"
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func (a *API) pickRulesID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "the Pick rules id must be a number")
		return 0, false
	}
	return id, true
}

// pickRulesError answers a refusal from the Pick rules editing calls.
func (a *API) pickRulesError(w http.ResponseWriter, r *http.Request, id int64, err error) {
	var fe *mediafiles.FieldError
	switch {
	case errors.As(err, &fe):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fe.Text, "field": fe.Field})
	case errors.Is(err, database.ErrNotFound):
		fail(w, http.StatusNotFound, "no such Pick rules")
	case errors.Is(err, library.ErrPickRulesName):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Other Pick rules already have this name.", "field": "name"})
	case errors.Is(err, library.ErrPresetLocked):
		fail(w, http.StatusConflict, "These are ready-made Pick rules: they keep their name and kinds and cannot be removed. Their rules may be changed, or make a copy.")
	case errors.Is(err, library.ErrPickRulesInUse):
		u, uerr := library.PickRulesUse(r.Context(), a.s.Store, id)
		if uerr != nil {
			internal(w, uerr)
			return
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  "These Pick rules are used by " + useWords(u) + ", so they cannot be removed, lose a kind in use, or switch between video and music.",
			"in_use": u})
	default:
		internal(w, err)
	}
}

func (a *API) getPickRules(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pickRulesID(w, r)
	if !ok {
		return
	}
	var p mediafiles.PickRules
	if err := database.Get(r.Context(), a.s.Store.Read(), "pick_rules", id, &p); err != nil {
		a.pickRulesError(w, r, id, err)
		return
	}
	u, err := library.PickRulesUse(r.Context(), a.s.Store, id)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detailOfPickRules(p, u))
}

const pickRulesHelp = `send {"name": "...", "kinds": ["movie"], "body": {...}}`

func (a *API) addPickRules(w http.ResponseWriter, r *http.Request) {
	var in library.PickRulesInput
	if !decode(w, r, &in, pickRulesHelp) {
		return
	}
	p, err := library.CreatePickRules(r.Context(), a.s.Store, in, actor(r))
	if err != nil {
		a.pickRulesError(w, r, 0, err)
		return
	}
	writeJSON(w, http.StatusCreated, detailOfPickRules(p, library.Use{}))
}

func (a *API) editPickRules(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pickRulesID(w, r)
	if !ok {
		return
	}
	var in library.PickRulesInput
	if !decode(w, r, &in, pickRulesHelp) {
		return
	}
	p, err := library.UpdatePickRules(r.Context(), a.s.Store, id, in, actor(r))
	if err != nil {
		a.pickRulesError(w, r, id, err)
		return
	}
	u, err := library.PickRulesUse(r.Context(), a.s.Store, id)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detailOfPickRules(p, u))
}

func (a *API) deletePickRules(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pickRulesID(w, r)
	if !ok {
		return
	}
	if err := library.DeletePickRules(r.Context(), a.s.Store, id, actor(r)); err != nil {
		a.pickRulesError(w, r, id, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
