package plugins

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Handlers serves the owner-only plugin routes under /api/v1/plugins.
type Handlers struct {
	// Engine runs the plugins; nil answers that plugins are not set up.
	Engine *Engine
	// LoginServer is the configured Headscale address, if any; its host is
	// the only sign-in host trusted besides login.tailscale.com.
	LoginServer string
}

// Routes registers the plugin routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/plugins", h.list)
	m.HandleFunc("POST /api/v1/plugins/{id}/install", sessionOnly(h.install))
	m.HandleFunc("POST /api/v1/plugins/{id}/start", sessionOnly(h.start))
	m.HandleFunc("POST /api/v1/plugins/{id}/stop", sessionOnly(h.stop))
	m.HandleFunc("DELETE /api/v1/plugins/{id}", sessionOnly(h.remove))
	m.HandleFunc("GET /api/v1/plugins/tailscale/status", h.tailscaleStatus)
	m.HandleFunc("POST /api/v1/plugins/tailscale/signout", sessionOnly(h.tailscaleSignOut))
}

// sessionOnly refuses every API key: an owner's admin key passes the scope
// gate, but running code on the server stays with the signed-in owner.
func sessionOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if who, ok := webserver.From(r.Context()); ok && who.KeyID != 0 {
			webserver.Fail(w, http.StatusForbidden, "plugins are changed only from a signed-in owner session, never with an API key")
			return
		}
		next(w, r)
	}
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

// tailscaleStatus returns the Tailscale plugin's status.json. The sign-in
// link is given only to the signed-in owner, never through an API key.
func (h *Handlers) tailscaleStatus(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	st, err := h.Engine.Status("tailscale")
	if err != nil {
		fail(w, err)
		return
	}
	if who, ok := webserver.From(r.Context()); ok && who.KeyID != 0 || !h.trustedLogin(st.LoginURL) {
		st.LoginURL = ""
	}
	type withQR struct {
		*TailscaleStatus
		LoginQR []string `json:"login_qr,omitempty"`
	}
	resp := withQR{TailscaleStatus: st}
	if st.LoginURL != "" {
		// A code to scan from a phone.
		resp.LoginQR = accounts.QRRows(st.LoginURL)
	}
	webserver.WriteJSON(w, http.StatusOK, resp)
}

// trustedLogin reports whether link is an https sign-in link on Tailscale's
// own host or the configured login server's host.
func (h *Handlers) trustedLogin(link string) bool {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "login.tailscale.com" {
		return true
	}
	if h.LoginServer == "" {
		return false
	}
	ls, err := url.Parse(h.LoginServer)
	return err == nil && ls.Hostname() != "" && strings.EqualFold(ls.Hostname(), host)
}

// tailscaleSignOut logs the device out, wipes its state and leaves the
// plugin stopped.
func (h *Handlers) tailscaleSignOut(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	remote, err := h.Engine.SignOut(r.Context(), "tailscale")
	if err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"signed_out": true, "removed_remotely": remote})
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
	case errors.Is(err, ErrBlocked), errors.Is(err, ErrNotOnList), errors.Is(err, ErrHashMismatch), errors.Is(err, ErrManifestHash),
		errors.Is(err, ErrManifest), errors.Is(err, ErrStaleBlock), errors.Is(err, ErrFileTooLarge),
		errors.Is(err, ErrTampered), errors.Is(err, ErrDisabled), errors.Is(err, ErrCounterOlder):
		webserver.Fail(w, http.StatusUnprocessableEntity, err.Error())
	default:
		webserver.Internal(w, "plugins", err)
	}
}
