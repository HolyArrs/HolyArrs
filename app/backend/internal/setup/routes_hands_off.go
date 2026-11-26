package setup

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// handsOffSetting is the body of the hands-off routes.
type handsOffSetting struct {
	On *bool `json:"on"`
}

// getHandsOff says whether hands-off mode is on.
func (h *PartHandlers) getHandsOff(w http.ResponseWriter, r *http.Request) {
	on := parts.HandsOff(r.Context(), h.Store.Read())
	webserver.WriteJSON(w, http.StatusOK, handsOffSetting{On: &on})
}

// putHandsOff turns hands-off mode on or off. Nothing is moved either way:
// titles keep their folders and files, and normal behaviour returns when
// it is turned off.
func (h *PartHandlers) putHandsOff(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	// A browser session confirms its password first; the first-run setup
	// session counts as confirmed until the wizard is done, so an owner
	// made with Plex, Jellyfin or Emby can choose this during setup.
	if who, ok := webserver.From(r.Context()); ok && who.Via == "session" && !accounts.RecentLocal(r) {
		webserver.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before changing hands-off mode", "code": "reauth"})
		return
	}
	var in handsOffSetting
	if !webserver.Decode(w, r, &in, `send {"on": true} or {"on": false}`) {
		return
	}
	if in.On == nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"on": true} or {"on": false}`)
		return
	}
	if err := parts.SetHandsOff(r.Context(), h.Store, h.Parts, *in.On, h.Actor(r)); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	what := "turned hands-off mode off"
	if *in.On {
		what = "turned hands-off mode on"
	}
	h.Event(r, "hands_off", "hands_off", what)
	webserver.WriteJSON(w, http.StatusOK, in)
}
