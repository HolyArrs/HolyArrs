package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// EventSettingsChanged is health's settings change Event kind.
const EventSettingsChanged = health.EventSettingsChanged

// settingsItems is each area's table and name column (never a secret).
var settingsItems = map[string]struct{ table, name string }{
	"pick-rules":      {"pick_rules", "name"},
	"library-folders": {"library_folders", "COALESCE(NULLIF(label, ''), path)"},
	"downloaders":     {"downloaders", "name"},
	"sources":         {"sources", "name"},
	"outside-lists":   {"outside_lists", "name"},
	"own-traits":      {"own_traits", "name"},
}

// settingsName is an item's name, or the newest item's when id is 0 (just
// added); "" when it is not known.
func (a *API) settingsName(ctx context.Context, area string, id int64) string {
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
	_ = a.s.Store.Read().QueryRowContext(ctx, q, args...).Scan(&name)
	return name
}

// logSettingsChanges logs each setup change through health.
func (a *API) logSettingsChanges(h http.Handler) http.Handler {
	return (&health.SettingsLog{Store: a.s.Store, Name: a.settingsName, Actor: actor, Who: whoName}).Wrap(h)
}
