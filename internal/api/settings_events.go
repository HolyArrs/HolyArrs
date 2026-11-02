package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
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
}

// settingsChange names the area and action of a request that changes
// setup, or ok false for any other request.
func settingsChange(method, path string) (area, label, action string, id int64, ok bool) {
	rest, found := strings.CutPrefix(path, "/api/v1/")
	if !found {
		return "", "", "", 0, false
	}
	seg := strings.Split(strings.Trim(rest, "/"), "/")
	if seg[0] == "settings" && len(seg) == 2 && method == http.MethodPut {
		return seg[1], "Settings: " + strings.ReplaceAll(seg[1], "-", " "), "changed", 0, true
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

// logSettingsChanges writes a settings_changed Event after each setup
// change that succeeded.
func (a *API) logSettingsChanges(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		area, label, action, id, ok := settingsChange(r.Method, r.URL.Path)
		if !ok {
			h.ServeHTTP(w, r)
			return
		}
		sw := &statusWriter{ResponseWriter: w}
		h.ServeHTTP(sw, r)
		if sw.code >= 300 {
			return
		}
		data := map[string]any{"area": area, "action": action}
		if id != 0 {
			data["id"] = id
		}
		_ = a.s.Store.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: actor(r), Kind: EventSettingsChanged,
			Summary: whoName(r) + " " + action + " " + label, Data: data})
	})
}
