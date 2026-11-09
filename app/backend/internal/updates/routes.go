package updates

import (
	"net/http"
	"net/url"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Handlers serves the update check's state and the "check now" button.
// HolyArrs never updates itself.
type Handlers struct {
	Checker *Checker
}

// Routes adds the update routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/app/update", h.get)
	m.HandleFunc("POST /api/v1/app/update/check", h.check)
}

// ViewFor shows the feed only to people who may change settings, and then
// as scheme, host and path only, never a name, password or query.
func ViewFor(r *http.Request, u UpdateInfo) UpdateInfo {
	if who, ok := webserver.From(r.Context()); ok && !who.Has(webserver.PermSettings) {
		u.Feed = ""
		return u
	}
	if p, err := url.Parse(u.Feed); err == nil {
		u.Feed = (&url.URL{Scheme: p.Scheme, Host: p.Host, Path: p.Path}).String()
	} else {
		u.Feed = ""
	}
	return u
}

// get answers the last answer; it never asks the feed.
func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, ViewFor(r, h.Checker.Update(r.Context())))
}

// check asks the feed now; nothing is asked while the check is off.
func (h *Handlers) check(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, ViewFor(r, h.Checker.CheckUpdate(r.Context())))
}
