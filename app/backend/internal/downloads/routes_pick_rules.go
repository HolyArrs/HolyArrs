package downloads

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
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

func (h *Handlers) pickRulesID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "the Pick rules id must be a number")
		return 0, false
	}
	return id, true
}

// pickRulesError answers a refusal from the Pick rules editing calls.
func (h *Handlers) pickRulesError(w http.ResponseWriter, r *http.Request, id int64, err error) {
	var fe *mediafiles.FieldError
	switch {
	case errors.As(err, &fe):
		webserver.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": fe.Text, "field": fe.Field})
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such Pick rules")
	case errors.Is(err, library.ErrPickRulesName):
		webserver.WriteJSON(w, http.StatusConflict, map[string]string{"error": "Other Pick rules already have this name.", "field": "name"})
	case errors.Is(err, library.ErrPresetLocked):
		webserver.Fail(w, http.StatusConflict, "These are ready-made Pick rules: they keep their name and kinds and cannot be removed. Their rules may be changed, or make a copy.")
	case errors.Is(err, library.ErrPickRulesInUse):
		u, uerr := library.PickRulesUse(r.Context(), h.Store, id)
		if uerr != nil {
			webserver.Internal(w, "downloads", uerr)
			return
		}
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":  "These Pick rules are used by " + useWords(u) + ", so they cannot be removed, lose a kind in use, or switch between video and music.",
			"in_use": u})
	default:
		webserver.Internal(w, "downloads", err)
	}
}

func (h *Handlers) getPickRules(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pickRulesID(w, r)
	if !ok {
		return
	}
	var p mediafiles.PickRules
	if err := database.Get(r.Context(), h.Store.Read(), "pick_rules", id, &p); err != nil {
		h.pickRulesError(w, r, id, err)
		return
	}
	u, err := library.PickRulesUse(r.Context(), h.Store, id)
	if err != nil {
		webserver.Internal(w, "downloads", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, detailOfPickRules(p, u))
}

const pickRulesHelp = `send {"name": "...", "kinds": ["movie"], "body": {...}}`

func (h *Handlers) addPickRules(w http.ResponseWriter, r *http.Request) {
	var in library.PickRulesInput
	if !webserver.Decode(w, r, &in, pickRulesHelp) {
		return
	}
	p, err := library.CreatePickRules(r.Context(), h.Store, in, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		h.pickRulesError(w, r, 0, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, detailOfPickRules(p, library.Use{}))
}

func (h *Handlers) editPickRules(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pickRulesID(w, r)
	if !ok {
		return
	}
	var in library.PickRulesInput
	if !webserver.Decode(w, r, &in, pickRulesHelp) {
		return
	}
	p, err := library.UpdatePickRules(r.Context(), h.Store, id, in, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		h.pickRulesError(w, r, id, err)
		return
	}
	h.recheckGoodEnough(r, p)
	u, err := library.PickRulesUse(r.Context(), h.Store, id)
	if err != nil {
		webserver.Internal(w, "downloads", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, detailOfPickRules(p, u))
}

// recheckGoodEnough queues the good enough re-check of the titles that use
// the saved set. The save already stands, so a failure is only logged.
func (h *Handlers) recheckGoodEnough(r *http.Request, p mediafiles.PickRules) {
	if err := h.Library.QueueRecheckPickRules(r.Context(), p, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1})); err != nil {
		log.Printf("downloads: queue the good enough re-check of pick rules %d: %v", p.ID, err)
	}
}

func (h *Handlers) deletePickRules(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pickRulesID(w, r)
	if !ok {
		return
	}
	if err := library.DeletePickRules(r.Context(), h.Store, id, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1})); err != nil {
		h.pickRulesError(w, r, id, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PickRulesView is a Pick rules set as choosers show it.
type PickRulesView struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Kinds  []string `json:"kinds"`
	Preset bool     `json:"preset"`
	// InUse counts every use (Versions, Library folders, Outside lists, ask
	// settings and Movies and TV defaults).
	InUse int `json:"in_use"`
}

func (h *Handlers) listPickRules(w http.ResponseWriter, r *http.Request) {
	ps, err := database.Select[mediafiles.PickRules](r.Context(), h.Store.Read(), "pick_rules", "1=1 ORDER BY id")
	if err != nil {
		webserver.Internal(w, "downloads", err)
		return
	}
	uses, err := library.PickRulesUses(r.Context(), h.Store)
	if err != nil {
		webserver.Internal(w, "downloads", err)
		return
	}
	out := []PickRulesView{}
	for _, p := range ps {
		k := p.Kinds
		if k == nil {
			k = []string{}
		}
		out = append(out, PickRulesView{ID: p.ID, Name: p.Name, Kinds: k, Preset: p.Preset, InUse: uses[p.ID].Total()})
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"pick_rules": out})
}
