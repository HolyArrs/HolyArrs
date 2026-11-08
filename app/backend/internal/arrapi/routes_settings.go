package arrapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Settings in the this_app group; missing means off.
const (
	settingOn         = "old_app_api"
	settingDeleteFile = "old_app_api_delete_files"
)

// settingsGroup is the settings group both switches live in.
const settingsGroup = "this_app"

// Handlers serves the Sonarr and Radarr prefixes and their settings routes.
type Handlers struct {
	Deps
	// Auth makes, lists and deletes the keys of the two sides; nil means
	// sign-in is off and no key can be made.
	Auth *auth.Auth
}

// Routes registers the two prefixes and their settings. Each prefix has one
// row per method so the permission, scope and part tables name it.
func (h *Handlers) Routes(r *webserver.Router) {
	son := Sonarr(h.deps())
	rad := Radarr(h.deps())
	r.Handle("GET /sonarr/", webserver.PermManageLibrary, son)
	r.Handle("POST /sonarr/", webserver.PermManageLibrary, son)
	r.Handle("PUT /sonarr/", webserver.PermManageLibrary, son)
	r.Handle("DELETE /sonarr/", webserver.PermManageLibrary, son)
	r.Handle("GET /radarr/", webserver.PermManageLibrary, rad)
	r.Handle("POST /radarr/", webserver.PermManageLibrary, rad)
	r.Handle("PUT /radarr/", webserver.PermManageLibrary, rad)
	r.Handle("DELETE /radarr/", webserver.PermManageLibrary, rad)
	r.Handle("GET /api/v1/settings/old-app-api", webserver.PermSettings, http.HandlerFunc(h.getOldAppAPI))
	r.Handle("PUT /api/v1/settings/old-app-api", webserver.PermSettings, http.HandlerFunc(h.putOldAppAPI))
	r.Handle("POST /api/v1/settings/old-app-api/keys/{side}", webserver.PermSettings, http.HandlerFunc(h.makeOldAppKey))
	r.Handle("DELETE /api/v1/settings/old-app-api/keys/{side}", webserver.PermSettings, http.HandlerFunc(h.deleteOldAppKey))
}

// deps is what the two adapters read and call; the delete-files switch is
// read from the settings.
func (h *Handlers) deps() Deps {
	d := h.Deps
	d.DeleteFiles = h.oldAppDeleteFiles
	return d
}

func (h *Handlers) switchOn(ctx context.Context, key string) bool {
	return switchValue(ctx, h.Store, key) == "on"
}

func (h *Handlers) On(ctx context.Context) bool { return h.switchOn(ctx, settingOn) }

// oldAppDeleteFiles says whether a delete from those apps may take the
// files too. Off by default: Seerr keeps its key in plain text, and with
// this off a leaked key can take titles out of the Library but no files.
func (h *Handlers) oldAppDeleteFiles(ctx context.Context) bool {
	return h.switchOn(ctx, settingDeleteFile)
}

// Gate answers 404 for both prefixes while the old-app API is off.
func (h *Handlers) Gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.CompatSide(r.URL.Path) != "" && !h.On(r.Context()) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// OldAppSide is one side as the settings show it.
type OldAppSide struct {
	Side string `json:"side"` // sonarr or radarr
	// Path is the address to paste into Seerr or Overseerr as the URL base
	// (with this server's host and port); URL is the whole address as this
	// request reached HolyArrs.
	Path string              `json:"path"`
	URL  string              `json:"url"`
	Key  *auth.CompatKeyView `json:"key,omitempty"` // nil: no key made yet
}

func (h *Handlers) oldAppAnswer(r *http.Request) (map[string]any, error) {
	var keys []auth.CompatKeyView
	if h.Auth != nil {
		var err error
		if keys, err = h.Auth.CompatKeys(r.Context()); err != nil {
			return nil, err
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	sides := []OldAppSide{}
	for _, s := range auth.CompatSides {
		v := OldAppSide{Side: s, Path: webserver.URLBaseFrom(r.Context()) + "/" + s}
		v.URL = scheme + "://" + r.Host + v.Path
		for i := range keys {
			if keys[i].Side == s {
				v.Key = &keys[i]
			}
		}
		sides = append(sides, v)
	}
	return map[string]any{"enabled": h.On(r.Context()), "delete_files": h.oldAppDeleteFiles(r.Context()), "sides": sides,
		"versions": map[string]string{"sonarr": SonarrVersion, "radarr": RadarrVersion}}, nil
}

func (h *Handlers) getOldAppAPI(w http.ResponseWriter, r *http.Request) {
	v, err := h.oldAppAnswer(r)
	if err != nil {
		webserver.Internal(w, "arrapi", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (h *Handlers) putOldAppAPI(w http.ResponseWriter, r *http.Request) {
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
	h.getOldAppAPI(w, r)
}

// SaveSwitches saves the old-app API's on/off and delete-files
// switches; a nil one is left as it is. Setup steps in this package (such
// as connecting Prowlarr) turn the API on through it.
func (h *Handlers) SaveSwitches(ctx context.Context, who domain.Actor, enabled, deleteFiles *bool) error {
	return saveSwitches(ctx, h.Store, who, map[string]*bool{settingOn: enabled, settingDeleteFile: deleteFiles})
}

// makeOldAppKey makes (or replaces) one side's key and shows it once. The
// key acts as the person who made it, so they must be allowed to manage
// the Library, and signed in with their password.
func (h *Handlers) makeOldAppKey(w http.ResponseWriter, r *http.Request) {
	side := r.PathValue("side")
	if side != "sonarr" && side != "radarr" {
		webserver.Fail(w, http.StatusNotFound, "the side is sonarr or radarr")
		return
	}
	if h.Auth == nil {
		webserver.Fail(w, http.StatusConflict, "sign-in is off, so no key can be made")
		return
	}
	if !auth.RequireStrong(w, r) {
		return
	}
	// The new key can change the Library, so a HolyArrs API key making it
	// must hold the write scope too: a key never makes one that reaches more.
	if who, ok := auth.From(r.Context()); ok && !who.HasScope(auth.ScopeWrite) {
		webserver.Fail(w, http.StatusForbidden, "this API key needs the write scope to make a key that changes the Library")
		return
	}
	if who, ok := auth.From(r.Context()); ok && !who.Has(auth.PermManageLibrary) {
		webserver.Fail(w, http.StatusForbidden, "the key acts as you, and your account may not manage the Library")
		return
	}
	k, err := h.Auth.MakeCompatKey(r, side)
	if errors.Is(err, auth.ErrNoCompatSide) {
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

func (h *Handlers) deleteOldAppKey(w http.ResponseWriter, r *http.Request) {
	side := r.PathValue("side")
	if h.Auth == nil || (side != "sonarr" && side != "radarr") {
		webserver.Fail(w, http.StatusNotFound, "no such key")
		return
	}
	if !auth.RequireStrong(w, r) {
		return
	}
	ok, err := h.Auth.DeleteCompatKey(r, side)
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
