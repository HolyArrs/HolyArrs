package accounts

import (
	"errors"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// ControlHandlers are the owner and admin controls (3.7.7): stages, linked
// apps and the whole app. They need the owner or an admin and, for an API
// key, the control scope (RouteScopes).
type ControlHandlers struct {
	// Parts runs the stages (nil: no stage controls).
	Parts *parts.Control
	// States holds the state of every supervised stage (may be nil).
	States *jobs.Registry
	// ExitApp stops the process for a whole-app restart (nil: cannot).
	ExitApp func(how string) error
	// Confirm runs the two-step confirm for what.
	Confirm func(w http.ResponseWriter, r *http.Request, what string) bool
	// Event writes the Event for a control.
	Event func(r *http.Request, action, target, summary string)
	// LinkTest is the test route for a linked app kind; unavailable is
	// set when that kind is not available here, both empty when unknown.
	LinkTest func(kind string) (test http.HandlerFunc, unavailable string)
}

// Routes mounts the controls.
func (h *ControlHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/stages/{name}/pause", h.stageControl("pause"))
	m.HandleFunc("POST /api/v1/stages/{name}/resume", h.stageControl("resume"))
	m.HandleFunc("POST /api/v1/stages/{name}/restart", h.stageControl("restart"))
	m.HandleFunc("POST /api/v1/links/{kind}/{id}/restart", h.linkRestart)
	m.HandleFunc("POST /api/v1/app/restart", h.appExit("restart"))
	m.HandleFunc("POST /api/v1/app/safe-mode", h.appExit("safe_mode"))
	m.HandleFunc("POST /api/v1/app/shutdown", h.appExit("shutdown"))
}

// whoName names the person for an Event summary.
func whoName(r *http.Request) string {
	if who, ok := webserver.From(r.Context()); ok && who.Name != "" {
		return who.Name
	}
	return "The owner"
}

func (h *ControlHandlers) stageControl(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !webserver.RequireStrong(w, r) {
			return
		}
		if h.Parts == nil {
			webserver.Fail(w, http.StatusServiceUnavailable, "stage controls are not available here")
			return
		}
		name := r.PathValue("name")
		var err error
		switch action {
		case "pause":
			err = h.Parts.Pause(name, whoName(r))
		case "resume":
			err = h.Parts.Resume(name)
		default:
			err = h.Parts.RestartStage(name)
		}
		if err != nil {
			ControlError(w, err)
			return
		}
		past := map[string]string{"pause": "paused", "resume": "resumed", "restart": "restarted"}[action]
		h.Event(r, "stage_"+action, name, past+" the stage "+name)
		out := map[string]any{"text": "The stage " + name + " was " + past + "."}
		if h.States != nil {
			if st, ok := h.States.Get("stage:" + name); ok {
				out["state"] = st
			}
		}
		webserver.WriteJSON(w, http.StatusOK, out)
	}
}

// linkRestart reconnects one linked app: a Downloader is signed in again
// and checked, a Source is tested now, a media server or subtitle site is
// tested again. Each answers what its own test answers. It needs the
// settings permission, like the test routes it runs, and writes its Event
// only when the linked app exists (an unknown id answers 404 first).
func (h *ControlHandlers) linkRestart(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	if who, ok := webserver.From(r.Context()); ok && !who.Has(webserver.PermSettings) {
		webserver.Fail(w, http.StatusForbidden, "your account may not do this; ask the owner")
		return
	}
	kind := r.PathValue("kind")
	test, unavailable := h.LinkTest(kind)
	if unavailable != "" {
		webserver.Fail(w, http.StatusServiceUnavailable, unavailable)
		return
	}
	if test == nil {
		webserver.Fail(w, http.StatusNotFound, "kind is downloader, source, media_server or subtitle_site")
		return
	}
	sw := &webserver.StatusWriter{ResponseWriter: w}
	test(sw, r)
	if sw.Code != http.StatusNotFound && sw.Code != http.StatusBadRequest {
		h.Event(r, "link_restart", kind+":"+r.PathValue("id"), "reconnected the "+strings.ReplaceAll(kind, "_", " ")+" "+r.PathValue("id"))
	}
}

// ControlError answers a refused control in plain words.
func ControlError(w http.ResponseWriter, err error) {
	var ne parts.NeedsError
	var oe parts.OffError
	switch {
	case errors.As(err, &ne), errors.As(err, &oe), errors.Is(err, parts.ErrSafeMode), errors.Is(err, parts.ErrBusy):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, parts.ErrNoStage), errors.Is(err, parts.ErrNoPart):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	default:
		webserver.Fail(w, http.StatusConflict, err.Error())
	}
}

// appExit is the whole-app restart, restart in safe mode and shut down.
// The process stops cleanly; Docker's restart policy starts it again.
func (h *ControlHandlers) appExit(how string) http.HandlerFunc {
	what := map[string]string{
		"restart":   "restart HolyArrs",
		"safe_mode": "restart HolyArrs in safe mode (only the core starts, once)",
		"shutdown":  "shut HolyArrs down",
	}[how]
	return func(w http.ResponseWriter, r *http.Request) {
		if !webserver.RequireStrong(w, r) {
			return
		}
		if h.ExitApp == nil {
			webserver.Fail(w, http.StatusServiceUnavailable, "this server cannot restart itself")
			return
		}
		if !h.Confirm(w, r, what) {
			return
		}
		text := map[string]string{
			"restart":   "HolyArrs stops now and Docker's restart policy starts it again in a few seconds, unless the policy is \"no\"; then start the container yourself.",
			"safe_mode": "HolyArrs stops now and starts again in safe mode through Docker's restart policy, unless the policy is \"no\"; then start the container yourself. The start after that is normal.",
			"shutdown":  "HolyArrs shuts down now. Docker's restart policy starts it again unless the policy is \"no\" (or the container is stopped by hand).",
		}[how]
		// ExitApp arranges the exit (and the safe-mode mark) and stops
		// the process shortly after, so the answer still reaches the page.
		if err := h.ExitApp(how); err != nil {
			webserver.Fail(w, http.StatusInternalServerError, "could not arrange safe mode; nothing was restarted (the log says why)")
			return
		}
		h.Event(r, "app_"+how, "app", map[string]string{"restart": "restarted HolyArrs", "safe_mode": "restarted HolyArrs in safe mode", "shutdown": "shut HolyArrs down"}[how])
		webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"text": text})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}
