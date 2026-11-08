package health

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
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

// settingsOne is the word for one item of each area in an Event summary.
var settingsOne = map[string]string{
	"pick-rules":      "Pick rule",
	"library-folders": "Library folder",
	"downloaders":     "Downloader",
	"sources":         "Source",
	"outside-lists":   "Outside list",
	"own-traits":      "trait",
}

// SettingsLog writes a settings_changed Event after each setup change that
// succeeded. Name is an item's name, or the newest item's when id is 0 (just
// added), "" when not known; Actor and Who are the caller and their name.
type SettingsLog struct {
	Store *database.Store
	Name  func(ctx context.Context, area string, id int64) string
	Actor func(*http.Request) domain.Actor
	Who   func(*http.Request) string
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

// Wrap logs the setup changes h serves.
func (l *SettingsLog) Wrap(h http.Handler) http.Handler {
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
			name = l.Name(r.Context(), area, id)
		}
		sw := &webserver.StatusWriter{ResponseWriter: w}
		h.ServeHTTP(sw, r)
		if sw.Code >= 300 {
			return
		}
		ctx := context.WithoutCancel(r.Context())
		if item && action != "deleted" {
			name = l.Name(ctx, area, id)
		}
		data := map[string]any{"area": area, "action": action}
		if id != 0 {
			data["id"] = id
		}
		summary := l.Who(r) + " " + action + " " + label
		if name != "" {
			data["name"] = name
			summary = settingsOne[area] + " \"" + name + "\" " + action + " by " + l.Who(r)
		}
		_ = l.Store.AddEvent(ctx, &domain.Event{Actor: l.Actor(r), Kind: EventSettingsChanged, Summary: summary, Data: data})
	})
}
