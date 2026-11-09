package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// TitleDefaultsHandlers serves the saved Movies and TV add defaults.
type TitleDefaultsHandlers struct {
	Store *database.Store
	// Actor is the signed-in person (the owner without sign-in).
	Actor func(r *http.Request) domain.Actor
}

// Routes registers the title-defaults settings routes.
func (h *TitleDefaultsHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/title-defaults", h.getTitleDefaults)
	m.HandleFunc("PUT /api/v1/settings/title-defaults", h.putTitleDefaults)
}

// KindDefaultsFor fills the fields an add left out from the saved Movies and
// TV defaults of kind (movie or show). Only keys that were saved count, and
// a saved folder or Pick rules that are gone fall back to the built-in choice.
func KindDefaultsFor(ctx context.Context, db *sql.DB, kind string, sent map[string]json.RawMessage,
	folder, rules *int64, keep **bool, search *bool) error {
	d, saved, err := library.ReadTitleDefaults(ctx, db)
	if err != nil {
		return err
	}
	kd := d.Movie
	if kind == "show" {
		kd = d.Show
	}
	if _, ok := sent["library_folder_id"]; !ok && saved[kind+"_library_folder_id"] && library.UsableDefault(ctx, db, "library_folders", kind, kd.LibraryFolderID) {
		*folder = kd.LibraryFolderID
	}
	if _, ok := sent["pick_rules_id"]; !ok && saved[kind+"_pick_rules_id"] && library.UsableDefault(ctx, db, "pick_rules", kind, kd.PickRulesID) {
		*rules = kd.PickRulesID
	}
	if _, ok := sent["keep_up_to_date"]; !ok && saved[kind+"_keep_up_to_date"] {
		v := kd.KeepUpToDate
		*keep = &v
	}
	if _, ok := sent["search_on_add"]; !ok && saved[kind+"_search_on_add"] {
		*search = kd.SearchOnAdd
	}
	return nil
}

func (h *TitleDefaultsHandlers) getTitleDefaults(w http.ResponseWriter, r *http.Request) {
	d, _, err := library.ReadTitleDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "requests", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}

func (h *TitleDefaultsHandlers) putTitleDefaults(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their value.
	d, _, err := library.ReadTitleDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "requests", err)
		return
	}
	const help = `send {"movie": {...}, "show": {...}} with any of library_folder_id, pick_rules_id, keep_up_to_date, search_on_add`
	sent, ok := webserver.DecodeKeys(w, r, &d, help)
	if !ok {
		return
	}
	// Only the fields sent are saved, so a left-out kind or field stays
	// unsaved and adds keep their own default for it.
	keys := []string{}
	for _, kind := range []string{"movie", "show"} {
		raw, ok := sent[kind]
		if !ok {
			continue
		}
		fields := map[string]json.RawMessage{}
		if json.Unmarshal(raw, &fields) != nil {
			webserver.Fail(w, http.StatusBadRequest, help)
			return
		}
		for f := range fields {
			keys = append(keys, kind+"_"+f)
		}
	}
	if err := library.SetTitleDefaults(r.Context(), h.Store, d, keys, h.Actor(r)); err != nil {
		switch {
		case errors.Is(err, library.ErrBadFolder), errors.Is(err, library.ErrNoPickRules), errors.Is(err, library.ErrNotVideoRules):
			webserver.Fail(w, http.StatusBadRequest, err.Error())
		default:
			webserver.Internal(w, "requests", err)
		}
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}
