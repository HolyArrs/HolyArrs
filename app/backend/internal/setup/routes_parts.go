package setup

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// PartHandlers are the part switches (3.7.6). They need the owner or an
// admin and, for an API key, the control scope (accounts.RouteScopes).
type PartHandlers struct {
	// Parts switches the parts (nil: every part on, no switches).
	Parts *parts.Control
	// SafeMode says the app started with only the core.
	SafeMode bool
	// Actor is who asks.
	Actor func(*http.Request) domain.Actor
	// Confirm runs the two-step confirm for what.
	Confirm func(w http.ResponseWriter, r *http.Request, what string) bool
	// Event writes the Event for a control.
	Event func(r *http.Request, action, target, summary string)
}

// Routes mounts the part switches.
func (h *PartHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/parts", h.listParts)
	m.HandleFunc("POST /api/v1/parts/{part}/on", h.partOn)
	m.HandleFunc("POST /api/v1/parts/{part}/off", h.partOff)
	m.HandleFunc("POST /api/v1/parts/{part}/restart", h.partRestart)
}

// PartOffBody is what a route of a part that is off answers.
func PartOffBody(o parts.Owner) map[string]string {
	name := "This part"
	if o == parts.Media {
		name = "Movies, TV shows and Music are all off, so this"
	} else if parts.Valid(parts.Part(o)) {
		name = parts.Part(o).Label()
	}
	return map[string]string{"error": "this part is off", "part": string(o),
		"text": name + " is off. The owner or an admin can turn it on again; its data is kept."}
}

// PartView is one part as the controls show it.
type PartView struct {
	Part   parts.Part `json:"part"`
	Name   string     `json:"name"`
	On     bool       `json:"on"`
	State  string     `json:"state"`
	Reason string     `json:"reason,omitempty"`
}

func (h *PartHandlers) listParts(w http.ResponseWriter, r *http.Request) {
	out := []PartView{}
	if h.Parts == nil {
		for _, p := range parts.All() {
			out = append(out, PartView{Part: p, Name: p.Label(), On: true, State: "running"})
		}
	} else {
		all := parts.All()
		for i, st := range h.Parts.Statuses() {
			p := all[i]
			out = append(out, PartView{Part: p, Name: st.Name, On: h.Parts.On(p), State: string(st.State), Reason: st.Reason})
		}
	}
	safeScreens, stages := []string{}, []string{}
	if h.Parts != nil {
		if h.Parts.SafeMode {
			safeScreens = parts.SafeScreens
		}
		stages = h.Parts.Stages()
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"parts": out, "screens": parts.Screens(), "safe_mode": h.SafeMode, "safe_screens": safeScreens, "stages": stages})
}

func (h *PartHandlers) partFrom(w http.ResponseWriter, r *http.Request) (parts.Part, bool) {
	if h.Parts == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "part switches are not available here")
		return "", false
	}
	p := parts.Part(r.PathValue("part"))
	if !parts.Valid(p) {
		webserver.Fail(w, http.StatusNotFound, parts.ErrNoPart.Error())
		return "", false
	}
	return p, true
}

func (h *PartHandlers) partOn(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	p, ok := h.partFrom(w, r)
	if !ok {
		return
	}
	if err := h.Parts.Set(r.Context(), p, true, h.Actor(r)); err != nil {
		accounts.ControlError(w, err)
		return
	}
	h.Event(r, "part_on", string(p), "turned "+p.Label()+" on")
	h.listParts(w, r)
}

func (h *PartHandlers) partOff(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	p, ok := h.partFrom(w, r)
	what := "turn " + p.Label() + " off (its screens hide, its scheduled work stops and its waiting Jobs wait; its data is kept)"
	if p == parts.Sources {
		what = "turn Sources off (HolyArrs stops searching on its own; a search you start on a title still runs, and Downloaders and imports keep working)"
	}
	if !ok || !h.Confirm(w, r, what) {
		return
	}
	if err := h.Parts.Set(r.Context(), p, false, h.Actor(r)); err != nil {
		accounts.ControlError(w, err)
		return
	}
	h.Event(r, "part_off", string(p), "turned "+p.Label()+" off")
	h.listParts(w, r)
}

func (h *PartHandlers) partRestart(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	p, ok := h.partFrom(w, r)
	if !ok || !h.Confirm(w, r, "restart "+p.Label()+" (running Jobs finish first; nothing is cut off)") {
		return
	}
	left, err := h.Parts.Restart(r.Context(), p)
	if err != nil {
		accounts.ControlError(w, err)
		return
	}
	h.Event(r, "part_restart", string(p), "restarted "+p.Label())
	text := p.Label() + " restarted."
	if left > 0 {
		text += " Some Jobs were still running after the wait; they carry on and finish, nothing was cut off."
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"text": text, "still_running": left})
}
