package plugins

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Handlers serves the owner-only plugin routes under /api/v1/plugins.
type Handlers struct {
	// Engine runs the plugins; nil answers that plugins are not set up.
	Engine *Engine
}

// Routes registers the plugin routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/plugins", h.list)
	m.HandleFunc("POST /api/v1/plugins/{id}/install", h.install)
	m.HandleFunc("POST /api/v1/plugins/{id}/start", h.start)
	m.HandleFunc("POST /api/v1/plugins/{id}/stop", h.stop)
	m.HandleFunc("DELETE /api/v1/plugins/{id}", h.remove)
}

func (h *Handlers) ready(w http.ResponseWriter) bool {
	if h.Engine == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "plugins are not set up in this build")
		return false
	}
	return true
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	ov, err := h.Engine.List(r.Context())
	if err != nil {
		webserver.Internal(w, "plugins", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ov)
}

func (h *Handlers) install(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	p, err := h.Engine.Install(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"plugin": p})
}

func (h *Handlers) start(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Engine.Start(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"started": true})
}

func (h *Handlers) stop(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Engine.Stop(r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"stopped": true})
}

func (h *Handlers) remove(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Engine.Remove(r.PathValue("id"), r.URL.Query().Get("keep") == "1"); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"removed": true})
}

// fail maps Engine refusals to answers that say what went wrong.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoKey):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotLinux):
		webserver.Fail(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, ErrNotInstalled), errors.Is(err, ErrNotListed):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrFetch), errors.Is(err, ErrListSignature), errors.Is(err, ErrListFormat),
		errors.Is(err, ErrListOlder), errors.Is(err, ErrListKind):
		webserver.Fail(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, ErrBlocked), errors.Is(err, ErrNotOnList), errors.Is(err, ErrHashMismatch),
		errors.Is(err, ErrManifest), errors.Is(err, ErrStaleBlock), errors.Is(err, ErrFileTooLarge),
		errors.Is(err, ErrTampered), errors.Is(err, ErrDisabled), errors.Is(err, ErrCounterOlder):
		webserver.Fail(w, http.StatusUnprocessableEntity, err.Error())
	default:
		webserver.Internal(w, "plugins", err)
	}
}
