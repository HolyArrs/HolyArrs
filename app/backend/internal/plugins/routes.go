package plugins

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
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
	m.HandleFunc("PUT /api/v1/plugins/file-lock", sessionOnly(h.fileLock))
	m.HandleFunc("PUT /api/v1/plugins/net-lock", sessionOnly(h.netLock))
	m.HandleFunc("PUT /api/v1/plugins/dev-mode", sessionOnly(h.devMode))
	m.HandleFunc("POST /api/v1/plugins/{id}/dev-install", sessionOnly(h.devInstall))
	m.HandleFunc("POST /api/v1/plugins/{id}/test-event", sessionOnly(h.testEvent))
	m.HandleFunc("PUT /api/v1/plugins/{id}/test-versions", sessionOnly(h.testVersions))
	m.HandleFunc("POST /api/v1/plugins/{id}/approve", sessionOnly(h.approve))
	m.HandleFunc("PUT /api/v1/plugins/{id}/owner-hosts", sessionOnly(h.ownerHosts))
	m.HandleFunc("DELETE /api/v1/plugins/{id}", sessionOnly(h.remove))
	m.HandleFunc("GET /api/v1/plugins/{id}/settings", sessionOnly(h.settings))
	m.HandleFunc("PUT /api/v1/plugins/{id}/settings", sessionOnly(h.saveSettings))
	m.HandleFunc("GET /api/v1/plugin-images/{path...}", h.image)
	// Remote access for any plugin the owner granted network.forward.
	m.HandleFunc("GET /api/v1/plugins/{id}/remote/status", h.remoteStatus)
	m.HandleFunc("GET /api/v1/plugins/{id}/activity", h.activity)
	m.HandleFunc("POST /api/v1/plugins/{id}/remote/signout", sessionOnly(h.remoteSignOut))
	m.HandleFunc("POST /api/v1/plugins/{id}/remote/name", sessionOnly(h.remoteName))
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
	// An optional body carries the folders the owner chose: {"folders": {"music": "/data/music"}}.
	var body ApproveInput
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
			webserver.Fail(w, http.StatusBadRequest, `send {"folders": {"folder_name": "/path"}} or {}`)
			return
		}
	}
	p, err := h.Engine.Install(r.Context(), r.PathValue("id"), body)
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

// fileLock sets, for the whole install, whether signed plugins may start
// without the file lock:
// {"allow_without": true|false}.
func (h *Handlers) fileLock(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var body struct {
		AllowWithout *bool `json:"allow_without"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.AllowWithout == nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"allow_without": true} or {"allow_without": false}`)
		return
	}
	if err := h.Engine.SetAllowNoFileLock(*body.AllowWithout); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"allow_without": *body.AllowWithout})
}

// netLock sets, for the whole install, whether plugins that list hosts may
// start without the network lock: {"allow_without": true|false}.
func (h *Handlers) netLock(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var body struct {
		AllowWithout *bool `json:"allow_without"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.AllowWithout == nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"allow_without": true} or {"allow_without": false}`)
		return
	}
	if err := h.Engine.SetAllowNoNetLock(*body.AllowWithout); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"allow_without": *body.AllowWithout})
}

// devMode turns developer mode on or off: {"on": true|false}.
func (h *Handlers) devMode(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var body struct {
		On *bool `json:"on"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.On == nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"on": true} or {"on": false}`)
		return
	}
	if err := h.Engine.SetDevMode(*body.On); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"on": *body.On})
}

// devInstall installs or reloads a plugin from the developer folder; the
// optional body is install's.
func (h *Handlers) devInstall(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var body ApproveInput
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
			webserver.Fail(w, http.StatusBadRequest, `send {"folders": {"folder_name": "/path"}} or {}`)
			return
		}
	}
	p, err := h.Engine.InstallDev(r.Context(), r.PathValue("id"), body)
	if err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"plugin": p})
}

// testEvent sends a developer plugin one made-up event: {"event": "library.add"}.
func (h *Handlers) testEvent(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var body struct {
		Event string `json:"event"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Event == "" {
		webserver.Fail(w, http.StatusBadRequest, `send {"event": "library.add"}`)
		return
	}
	if err := h.Engine.TestEvent(r.PathValue("id"), body.Event); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"sent": true})
}

// testVersions sets whether a plugin gets test versions: {"on": true|false}.
func (h *Handlers) testVersions(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var body struct {
		On *bool `json:"on"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.On == nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"on": true} or {"on": false}`)
		return
	}
	if err := h.Engine.SetTestVersions(r.PathValue("id"), *body.On); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"on": *body.On})
}

// approve records the owner's OK for what the installed version asks for:
// {"folders": {"<name>": "<folder>" | ""}, "owner_hosts": [...]}.
func (h *Handlers) approve(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var body ApproveInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"folders": {"folder_name": "/path"}}`)
		return
	}
	if err := h.Engine.Approve(r.PathValue("id"), body); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"approved": true})
}

// ownerHosts saves the hosts the owner added for a plugin: {"hosts": ["hs.example.com:8080"]}.
func (h *Handlers) ownerHosts(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var body struct {
		Hosts []string `json:"hosts"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"hosts": ["host_name:port"]}`)
		return
	}
	if err := h.Engine.SetOwnerHosts(r.PathValue("id"), body.Hosts); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (h *Handlers) remove(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if err := h.Engine.Remove(r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"removed": true})
}

// settings shows a plugin's declared fields and values; secrets show only as set.
func (h *Handlers) settings(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	v, err := h.Engine.Settings(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

// saveSettings stores a plugin's settings and restarts it when it runs.
func (h *Handlers) saveSettings(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var in SettingsInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		webserver.Fail(w, http.StatusBadRequest, "settings must be a JSON object")
		return
	}
	serr := h.Engine.SaveSettings(r.PathValue("id"), in)
	if serr != nil && !errors.Is(serr, ErrRestart) {
		fail(w, serr)
		return
	}
	v, err := h.Engine.Settings(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	if serr != nil {
		v.StartError = serr.Error() // saved; the page says the plugin did not start again
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

// statusOf answers a forwarding plugin's status.json.
func (h *Handlers) statusOf(w http.ResponseWriter, r *http.Request, id string) {
	if !h.ready(w) {
		return
	}
	st, err := h.Engine.Status(id)
	if err != nil {
		fail(w, err)
		return
	}
	if who, ok := webserver.From(r.Context()); ok && who.KeyID != 0 || !h.trustedLoginFor(id, st.LoginURL) {
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

func (h *Handlers) signOutOf(w http.ResponseWriter, r *http.Request, id string) {
	if !h.ready(w) {
		return
	}
	remote, err := h.Engine.SignOut(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"signed_out": true, "removed_remotely": remote})
}

func (h *Handlers) nameOf(w http.ResponseWriter, r *http.Request, id string) {
	if !h.ready(w) {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		webserver.Fail(w, http.StatusBadRequest, "send {\"name\": \"...\"}")
		return
	}
	if err := h.Engine.SetDeviceName(id, in.Name); err != nil {
		if errors.Is(err, ErrBadName) {
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"saved": true})
}

// fail maps Engine refusals to answers that say what went wrong.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoKey), errors.Is(err, ErrPublisherChanged), errors.Is(err, ErrPublisherUnknown):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotLinux):
		webserver.Fail(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, ErrNotInstalled), errors.Is(err, ErrNotListed), errors.Is(err, ErrDevMissing):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrFetch), errors.Is(err, ErrListSignature), errors.Is(err, ErrListFormat),
		errors.Is(err, ErrListOlder), errors.Is(err, ErrListKind):
		webserver.Fail(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, ErrBlocked), errors.Is(err, ErrNotOnList), errors.Is(err, ErrHashMismatch), errors.Is(err, ErrManifestHash),
		errors.Is(err, ErrManifest), errors.Is(err, ErrStaleBlock), errors.Is(err, ErrFileTooLarge),
		errors.Is(err, ErrTampered), errors.Is(err, ErrDisabled), errors.Is(err, ErrSettings), errors.Is(err, ErrNeedsNewerApp), errors.Is(err, ErrCounterOlder), errors.Is(err, ErrNoLandlock),
		errors.Is(err, ErrNoNetLock), errors.Is(err, ErrNeedsApproval), errors.Is(err, ErrResources), errors.Is(err, ErrFolderMoved), errors.Is(err, ErrFolderRefused), errors.Is(err, ErrDevOff), errors.Is(err, ErrDevSigned):
		webserver.Fail(w, http.StatusUnprocessableEntity, err.Error())
	default:
		webserver.Internal(w, "plugins", err)
	}
}

// forwarder is the {id} of a remote-access route when the owner granted it
// network.forward; it answers 404 itself when not.
func (h *Handlers) forwarder(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !h.ready(w) {
		return "", false
	}
	id := r.PathValue("id")
	if !slices.Contains(h.Engine.GrantedScopes(id), "network.forward") {
		webserver.Fail(w, http.StatusNotFound, "this plugin does not give remote access")
		return "", false
	}
	return id, true
}

func (h *Handlers) remoteStatus(w http.ResponseWriter, r *http.Request) {
	if id, ok := h.forwarder(w, r); ok {
		h.statusOf(w, r, id)
	}
}

func (h *Handlers) remoteSignOut(w http.ResponseWriter, r *http.Request) {
	if id, ok := h.forwarder(w, r); ok {
		h.signOutOf(w, r, id)
	}
}

func (h *Handlers) remoteName(w http.ResponseWriter, r *http.Request) {
	if id, ok := h.forwarder(w, r); ok {
		h.nameOf(w, r, id)
	}
}

// trustedLoginFor: Tailscale's sign-in hosts for Tailscale; for another
// forwarder, an https link on one of the hosts its reviewed manifest lists.
func (h *Handlers) trustedLoginFor(id, link string) bool {
	if h.trustedLogin(link) {
		return true
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	m := h.Engine.readManifest(id)
	if m == nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, p := range m.Access.Hosts {
		if matchHost([]string{strings.ToLower(p)}, host) {
			return true
		}
	}
	return false
}

// activity is a plugin's recent calls to HolyArrs: allowed, refused and over
// its limits, newest first.
func (h *Handlers) activity(w http.ResponseWriter, r *http.Request) {
	calls, refused, limited := accounts.PluginActivity(r.PathValue("id"))
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"calls": calls, "refused": refused, "limited": limited})
}
