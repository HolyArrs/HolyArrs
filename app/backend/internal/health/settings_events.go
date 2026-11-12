package health

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// EventSettingsChanged is one change to settings, pick rules, Library
// folders, Downloaders, Sources or Outside lists: the area, what was done
// and who did it. It never holds the values sent (they may be secrets).
const EventSettingsChanged = "settings_changed"

// settingsAreas are the setup lists whose add (POST /x), change (PUT
// /x/{id}) and delete (DELETE /x/{id}) are logged.
var settingsAreas = map[string]string{
	"pick-rules":      "Pick rules",
	"library-folders": "Library folders",
	"downloaders":     "Downloaders",
	"sources":         "Sources",
	"outside-lists":   "Outside lists",
	"own-traits":      "Your traits",
}

// settingsItems is each area's table, the name column (never a secret)
// and the word for one item in an Event summary.
var settingsItems = map[string]struct{ table, name, one string }{
	"pick-rules":      {"pick_rules", "name", "Pick rule"},
	"library-folders": {"library_folders", "COALESCE(NULLIF(label, ''), path)", "Library folder"},
	"downloaders":     {"downloaders", "name", "Downloader"},
	"sources":         {"sources", "name", "Source"},
	"outside-lists":   {"outside_lists", "name", "Outside list"},
	"own-traits":      {"own_traits", "name", "trait"},
}

// SettingsLog writes the settings_changed Events. Who names the person for
// a summary and Actor is who did it; both come from the caller.
type SettingsLog struct {
	Store *database.Store
	Who   func(*http.Request) string
	Actor func(*http.Request) domain.Actor
}

// Name is an item's name, or the newest item's when id is 0 (just
// added); "" when it is not known.
func (a *SettingsLog) Name(ctx context.Context, area string, id int64) string {
	it, ok := settingsItems[area]
	if !ok {
		return ""
	}
	q := "SELECT " + it.name + " FROM " + it.table + " WHERE id = ?"
	args := []any{id}
	if id == 0 {
		q, args = "SELECT "+it.name+" FROM "+it.table+" ORDER BY id DESC LIMIT 1", nil
	}
	var name string
	_ = a.Store.Read().QueryRowContext(ctx, q, args...).Scan(&name)
	return name
}

// SettingsChange names the area and action of a request that changes
// setup, or ok false for any other request.
func SettingsChange(method, path string) (area, label, action string, id int64, ok bool) {
	rest, found := strings.CutPrefix(path, "/api/v1/")
	if !found {
		return "", "", "", 0, false
	}
	seg := strings.Split(strings.Trim(rest, "/"), "/")
	if seg[0] == "settings" && len(seg) == 2 && method == http.MethodPut {
		return seg[1], "Settings: " + strings.ReplaceAll(seg[1], "-", " "), "changed", 0, true
	}
	// Sign-in settings: local-network skip, two-factor rules, password length, outside sign-in.
	if seg[0] == "auth" && len(seg) == 2 && seg[1] == "settings" && method == http.MethodPut {
		return "sign-in", "Sign-in settings", "changed", 0, true
	}
	label, known := settingsAreas[seg[0]]
	if !known {
		return "", "", "", 0, false
	}
	// A Library folder's new path is logged like its other edits.
	if seg[0] == "library-folders" && len(seg) == 3 && seg[2] == "path" && method == http.MethodPut {
		seg = seg[:2]
	}
	switch {
	case len(seg) == 1 && method == http.MethodPost:
		action = "added"
	case len(seg) == 2 && method == http.MethodPut:
		action = "changed"
	case len(seg) == 2 && method == http.MethodDelete:
		action = "deleted"
	default:
		return "", "", "", 0, false
	}
	if len(seg) == 2 {
		if id, _ = strconv.ParseInt(seg[1], 10, 64); id == 0 {
			return "", "", "", 0, false
		}
	}
	return seg[0], label, action, id, true
}

// Wrap writes a settings_changed Event after each setup change that
// succeeded.
func (a *SettingsLog) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		area, label, action, id, ok := SettingsChange(r.Method, r.URL.Path)
		if !ok {
			h.ServeHTTP(w, r)
			return
		}
		// Only a list item has a name; a whole settings page (PUT
		// /settings/downloaders) is not "the newest Downloader".
		item := !strings.HasPrefix(label, "Settings: ") && area != "sign-in"
		name := ""
		if item && action == "deleted" {
			name = a.Name(r.Context(), area, id)
		}
		sw := &webserver.StatusWriter{ResponseWriter: w}
		h.ServeHTTP(sw, r)
		if sw.Code >= 300 {
			return
		}
		ctx := context.WithoutCancel(r.Context())
		if item && action != "deleted" {
			name = a.Name(ctx, area, id)
		}
		data := map[string]any{"area": area, "action": action}
		if id != 0 {
			data["id"] = id
		}
		summary := a.Who(r) + " " + action + " " + label
		if name != "" {
			data["name"] = name
			summary = settingsItems[area].one + " \"" + name + "\" " + action + " by " + a.Who(r)
		}
		_ = a.Store.AddEvent(ctx, &domain.Event{Actor: a.Actor(r), Kind: EventSettingsChanged, Summary: summary, Data: data})
	})
}
