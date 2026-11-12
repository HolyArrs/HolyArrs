package arrapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Setting keys in the this_app group. A missing key means off.
const (
	settingOn         = "old_app_api"
	settingDeleteFile = "old_app_api_delete_files"
)

// settingsGroup is the settings group that holds both switches.
const settingsGroup = "this_app"

// Handlers serves the Sonarr and Radarr prefixes and their settings
// routes. The prefixes answer 404 until the owner turns the arr API on.
// Each side has its own key, so one leaked key reaches only one side.
type Handlers struct {
	Deps
	// Auth makes, lists and deletes the keys of both sides. Nil means
	// sign-in is off and no key can be made.
	Auth *accounts.Auth

	hooks *hookSender
}

// Watch sends each import the Library records to the webhook records
// those apps keep, on goroutines Wait waits for. Ending ctx stops them.
func (h *Handlers) Watch(ctx context.Context) {
	h.hooks = &hookSender{store: h.Store, vault: h.Secrets, allowed: h.maySend}
	h.hooks.watch(ctx)
}

// Wait waits for the sends Watch started.
func (h *Handlers) Wait() {
	if h.hooks != nil {
		h.hooks.wait()
	}
}

// maySend reports whether a side may tell its hooks: the arr API is on
// and the side still has a key. Taking a side's key away stops its hooks.
func (h *Handlers) maySend(ctx context.Context, side string) bool {
	if h.Auth == nil || !h.On(ctx) {
		return false
	}
	keys, err := h.Auth.ArrKeys(ctx)
	if err != nil {
		return false
	}
	for _, k := range keys {
		if k.Side == side {
			return true
		}
	}
	return false
}

// Routes registers the two prefixes and their settings. Each prefix gets
// one row per method so the permission, scope and part tables can name it.
func (h *Handlers) Routes(m *webserver.Router) {
	son := Sonarr(h.deps())
	rad := Radarr(h.deps())
	m.Handle("GET /sonarr/", webserver.PermManageLibrary, son)
	m.Handle("POST /sonarr/", webserver.PermManageLibrary, son)
	m.Handle("PUT /sonarr/", webserver.PermManageLibrary, son)
	m.Handle("DELETE /sonarr/", webserver.PermManageLibrary, son)
	m.Handle("GET /radarr/", webserver.PermManageLibrary, rad)
	m.Handle("POST /radarr/", webserver.PermManageLibrary, rad)
	m.Handle("PUT /radarr/", webserver.PermManageLibrary, rad)
	m.Handle("DELETE /radarr/", webserver.PermManageLibrary, rad)
	m.Handle("GET /api/v1/settings/old-app-api", webserver.PermSettings, http.HandlerFunc(h.getSettings))
	m.Handle("PUT /api/v1/settings/old-app-api", webserver.PermSettings, http.HandlerFunc(h.putSettings))
	m.Handle("POST /api/v1/settings/old-app-api/keys/{side}", webserver.PermSettings, http.HandlerFunc(h.makeKey))
	m.Handle("DELETE /api/v1/settings/old-app-api/keys/{side}", webserver.PermSettings, http.HandlerFunc(h.deleteKey))
}

// deps returns what the two adapters read and call. The delete-files
// switch is read from the settings on each use.
func (h *Handlers) deps() Deps {
	d := h.Deps
	d.DeleteFiles = h.deleteFilesOn
	return d
}

func (h *Handlers) switchOn(ctx context.Context, key string) bool {
	return switchValue(ctx, h.Store, key) == "on"
}

// On reports whether the arr API is switched on.
func (h *Handlers) On(ctx context.Context) bool { return h.switchOn(ctx, settingOn) }

// deleteFilesOn reports whether a delete from those apps may take the
// files too. It is off by default because Seerr keeps its key in plain
// text. With it off, a leaked key can remove titles but never files.
func (h *Handlers) deleteFilesOn(ctx context.Context) bool {
	return h.switchOn(ctx, settingDeleteFile)
}

// Gate answers 404 for both prefixes while the arr API is off.
func (h *Handlers) Gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accounts.ArrSide(r.URL.Path) != "" && !h.On(r.Context()) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Side is one side as the settings page shows it.
type Side struct {
	Side string `json:"side"` // sonarr or radarr
	// Path is the URL base to paste into Seerr or Overseerr, next to this
	// server's host and port. URL is the whole address as this request
	// reached HolyArrs.
	Path string               `json:"path"`
	URL  string               `json:"url"`
	Key  *accounts.ArrKeyView `json:"key,omitempty"` // nil until a key is made
}

func (h *Handlers) answer(r *http.Request) (map[string]any, error) {
	var keys []accounts.ArrKeyView
	if h.Auth != nil {
		var err error
		if keys, err = h.Auth.ArrKeys(r.Context()); err != nil {
			return nil, err
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	sides := []Side{}
	for _, s := range accounts.ArrSides {
		v := Side{Side: s, Path: webserver.URLBaseFrom(r.Context()) + "/" + s}
		v.URL = scheme + "://" + r.Host + v.Path
		for i := range keys {
			if keys[i].Side == s {
				v.Key = &keys[i]
			}
		}
		sides = append(sides, v)
	}
	return map[string]any{"enabled": h.On(r.Context()), "delete_files": h.deleteFilesOn(r.Context()), "sides": sides,
		"versions": map[string]string{"sonarr": SonarrVersion, "radarr": RadarrVersion}}, nil
}

func (h *Handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	v, err := h.answer(r)
	if err != nil {
		webserver.Internal(w, "arrapi", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (h *Handlers) putSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled     *bool `json:"enabled"`
		DeleteFiles *bool `json:"delete_files"`
	}
	if !webserver.Decode(w, r, &in, `send {"enabled": true} and/or {"delete_files": false}`) {
		return
	}
	if err := h.SaveSwitches(r.Context(), actor(r), in.Enabled, in.DeleteFiles); err != nil {
		webserver.Internal(w, "arrapi", err)
		return
	}
	h.getSettings(w, r)
}

// SaveSwitches saves the arr API's on/off and delete-files switches. A
// nil switch is left as it is. Setup steps such as connecting Prowlarr
// turn the API on through it.
func (h *Handlers) SaveSwitches(ctx context.Context, who domain.Actor, enabled, deleteFiles *bool) error {
	return saveSwitches(ctx, h.Store, who, map[string]*bool{settingOn: enabled, settingDeleteFile: deleteFiles})
}

// makeKey makes or replaces one side's key and shows it once. The key
// acts as the person who made it, so that person must be allowed to
// manage the Library and must have signed in with a password.
func (h *Handlers) makeKey(w http.ResponseWriter, r *http.Request) {
	side := r.PathValue("side")
	if side != "sonarr" && side != "radarr" {
		webserver.Fail(w, http.StatusNotFound, "the side is sonarr or radarr")
		return
	}
	if h.Auth == nil {
		webserver.Fail(w, http.StatusConflict, "sign-in is off, so no key can be made")
		return
	}
	if !webserver.RequireStrong(w, r) {
		return
	}
	// The new key can change the Library. A HolyArrs API key making it must
	// hold the write scope too, so no key can make one that reaches more.
	if who, ok := webserver.From(r.Context()); ok && !who.HasScope(webserver.ScopeWrite) {
		webserver.Fail(w, http.StatusForbidden, "this API key needs the write scope to make a key that changes the Library")
		return
	}
	if who, ok := webserver.From(r.Context()); ok && !who.Has(webserver.PermManageLibrary) {
		webserver.Fail(w, http.StatusForbidden, "the key acts as you, and your account may not manage the Library")
		return
	}
	k, err := h.Auth.MakeArrKey(r, side)
	if errors.Is(err, accounts.ErrNoArrSide) {
		webserver.Fail(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "arrapi", err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"side": side, "key": k, "path": webserver.URLBaseFrom(r.Context()) + "/" + side,
		"note": "This key is shown only now. Paste it into Seerr or Overseerr as the API key; an older key for this side no longer works."})
}

func (h *Handlers) deleteKey(w http.ResponseWriter, r *http.Request) {
	side := r.PathValue("side")
	if h.Auth == nil || (side != "sonarr" && side != "radarr") {
		webserver.Fail(w, http.StatusNotFound, "no such key")
		return
	}
	if !webserver.RequireStrong(w, r) {
		return
	}
	ok, err := h.Auth.DeleteArrKey(r, side)
	if err != nil {
		webserver.Internal(w, "arrapi", err)
		return
	}
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
