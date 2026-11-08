package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// decodeKeys is decode that also answers which top-level keys the body
// holds, so a left-out field can take a saved default.
func decodeKeys(w http.ResponseWriter, r *http.Request, v any, help string) (map[string]json.RawMessage, bool) {
	return webserver.DecodeKeys(w, r, v, help)
}

// kindDefaultsFor fills the fields an add left out from the saved Movies and
// TV defaults of kind (movie or show). Only keys that were saved count, and
// a saved folder or Pick rules that are gone fall back to the built-in choice.
func (a *API) kindDefaultsFor(ctx context.Context, kind string, sent map[string]json.RawMessage,
	folder, rules *int64, keep **bool, search *bool) error {
	d, saved, err := library.ReadTitleDefaults(ctx, a.s.Store.Read())
	if err != nil {
		return err
	}
	kd := d.Movie
	if kind == "show" {
		kd = d.Show
	}
	db := a.s.Store.Read()
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

func (a *API) getTitleDefaults(w http.ResponseWriter, r *http.Request) {
	d, _, err := library.ReadTitleDefaults(r.Context(), a.s.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}

func (a *API) putTitleDefaults(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their value.
	d, _, err := library.ReadTitleDefaults(r.Context(), a.s.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	const help = `send {"movie": {...}, "show": {...}} with any of library_folder_id, pick_rules_id, keep_up_to_date, search_on_add`
	sent, ok := decodeKeys(w, r, &d, help)
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
	if err := library.SetTitleDefaults(r.Context(), a.s.Store, d, keys, actor(r)); err != nil {
		switch {
		case errors.Is(err, library.ErrBadFolder), errors.Is(err, library.ErrNoPickRules), errors.Is(err, library.ErrNotVideoRules):
			webserver.Fail(w, http.StatusBadRequest, err.Error())
		default:
			webserver.Internal(w, "api", err)
		}
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}
