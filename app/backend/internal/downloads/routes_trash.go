package downloads

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TrashClient reads TRaSH Guides; tests point it at a local server.
var TrashClient = library.NewTrashGuides()

// listTrashProfiles answers the TRaSH Guides profiles for ?app=sonarr
// (shows) or radarr (movies).
func (h *Handlers) listTrashProfiles(w http.ResponseWriter, r *http.Request) {
	app := r.URL.Query().Get("app")
	if app != "sonarr" && app != "radarr" {
		webserver.Fail(w, http.StatusBadRequest, "app must be sonarr (shows) or radarr (movies)")
		return
	}
	d, err := TrashClient.Load(r.Context(), app)
	if err != nil {
		webserver.Fail(w, http.StatusBadGateway, err.Error())
		return
	}
	out := d.Profiles
	if out == nil {
		out = []library.TrashProfile{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

const trashHelp = `send {"app": "sonarr" or "radarr", "profile_id": "<TRaSH profile id>"}`

// applyTrash applies a TRaSH Guides profile to Pick rules {id}. Sending it
// again re-syncs: synced traits are updated, never made twice.
func (h *Handlers) applyTrash(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pickRulesID(w, r)
	if !ok {
		return
	}
	var in struct {
		App       string `json:"app"`
		ProfileID string `json:"profile_id"`
	}
	if !webserver.Decode(w, r, &in, trashHelp) {
		return
	}
	if in.App != "sonarr" && in.App != "radarr" {
		webserver.Fail(w, http.StatusBadRequest, "app must be sonarr (shows) or radarr (movies)")
		return
	}
	res, err := library.ApplyTrash(r.Context(), h.Store, TrashClient, id, in.App, in.ProfileID, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	var fe *mediafiles.FieldError
	switch {
	case err == nil:
	case errors.Is(err, library.ErrTrashProfile):
		webserver.Fail(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, database.ErrNotFound), errors.As(err, &fe), errors.Is(err, library.ErrOwnTraitName):
		h.pickRulesError(w, r, id, err)
		return
	default:
		webserver.Fail(w, http.StatusBadGateway, err.Error())
		return
	}
	h.recheckGoodEnough(r, res.PickRules)
	u, err := library.PickRulesUse(r.Context(), h.Store, id)
	if err != nil {
		webserver.Internal(w, "downloads", err)
		return
	}
	if res.Scored == nil {
		res.Scored = []library.TrashScored{}
	}
	if res.Skipped == nil {
		res.Skipped = []library.TrashSkipped{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{
		"pick_rules": detailOfPickRules(res.PickRules, u),
		"added":      res.Added, "updated": res.Updated,
		"scored": res.Scored, "skipped": res.Skipped,
	})
}
