package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/compat"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The old-app API (roadmap #85, decision 0008): Seerr and Overseerr add
// shows and movies to HolyArrs as if it were Sonarr (/sonarr/) and Radarr
// (/radarr/), with keys of their own. It is off until turned on here;
// while off both prefixes answer 404 before anything else looks at them.

// Settings in the this_app group; missing means off.
const (
	settingOldAppAPI    = "old_app_api"
	settingOldAppDelete = "old_app_api_delete_files"
)

// oldAppRoutes are the two prefixes and their settings. Each prefix has one
// row per method so the permission, scope and part tables name it.
func (a *API) oldAppRoutes(m *http.ServeMux) {
	son := compat.Sonarr(a.compatDeps())
	rad := compat.Radarr(a.compatDeps())
	m.Handle("GET /sonarr/", son)
	m.Handle("POST /sonarr/", son)
	m.Handle("PUT /sonarr/", son)
	m.Handle("DELETE /sonarr/", son)
	m.Handle("GET /radarr/", rad)
	m.Handle("POST /radarr/", rad)
	m.Handle("PUT /radarr/", rad)
	m.Handle("DELETE /radarr/", rad)
	m.HandleFunc("GET /api/v1/settings/old-app-api", a.getOldAppAPI)
	m.HandleFunc("PUT /api/v1/settings/old-app-api", a.putOldAppAPI)
	m.HandleFunc("POST /api/v1/settings/old-app-api/keys/{side}", a.makeOldAppKey)
	m.HandleFunc("DELETE /api/v1/settings/old-app-api/keys/{side}", a.deleteOldAppKey)
}

func (a *API) compatDeps() compat.Deps {
	return compat.Deps{Store: a.s.Store, Library: a.s.Library, Metadata: a.s.Metadata, Queue: a.s.Queue, DeleteFiles: a.oldAppDeleteFiles,
		Picture: a.oldAppPicture, Secrets: a.s.Secrets, SourceSaved: a.publishSource, SourceRemoved: a.forgetSource}
}

// forgetSource drops what the searcher keeps of a removed Source, as the
// Sources page's delete does.
func (a *API) forgetSource(id int64) {
	if a.s.Search != nil {
		a.s.Search.ForgetSource(id)
	}
}

// oldAppPicture sends a cover for those apps (Bazarr shows them): a
// stored poster ("poster:<cache name>"), made smaller once when px asks,
// or a wide picture's address, through the same cache and miss budget as
// the TMDB pictures of the web app.
func (a *API) oldAppPicture(w http.ResponseWriter, r *http.Request, src string, px int) {
	if a.s.Library == nil || a.s.Library.Artwork == nil {
		http.NotFound(w, r)
		return
	}
	c := a.s.Library.Artwork
	name, ok := strings.CutPrefix(src, "poster:")
	if !ok {
		a.serveRemoteArtwork(w, r, src, serveCover)
		return
	}
	p, ok := c.Path(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if px > 0 {
		if n, sp, err := c.Smaller(name, px); err == nil && n != name {
			serveArtworkCached(w, r, n, sp, "private, max-age=86400")
			return
		}
	}
	serveCover(w, r, name, p)
}

// serveCover sends a cover. Its address stays the same when the title's
// picture changes, so it is kept a day and then revalidated by name,
// never kept a year as the web app's own picture addresses are.
func serveCover(w http.ResponseWriter, r *http.Request, name, p string) {
	if library.StableFile(p) {
		serveArtwork(w, r, name, p) // already a day, named by the file's time
		return
	}
	serveArtworkCached(w, r, name, p, "private, max-age=86400")
}

func (a *API) thisAppOn(ctx context.Context, key string) bool {
	var v string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
	return v == "on"
}

func (a *API) oldAppOn(ctx context.Context) bool { return a.thisAppOn(ctx, settingOldAppAPI) }

// oldAppDeleteFiles says whether a delete from those apps may take the
// files too. Off by default: Seerr keeps its key in plain text, and with
// this off a leaked key can take titles out of the Library but no files.
func (a *API) oldAppDeleteFiles(ctx context.Context) bool {
	return a.thisAppOn(ctx, settingOldAppDelete)
}

// oldAppGate answers 404 for both prefixes while the old-app API is off.
func (a *API) oldAppGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.CompatSide(r.URL.Path) != "" && !a.oldAppOn(r.Context()) {
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

func (a *API) oldAppAnswer(r *http.Request) (map[string]any, error) {
	var keys []auth.CompatKeyView
	if a.s.Auth != nil {
		var err error
		if keys, err = a.s.Auth.CompatKeys(r.Context()); err != nil {
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
	return map[string]any{"enabled": a.oldAppOn(r.Context()), "delete_files": a.oldAppDeleteFiles(r.Context()), "sides": sides,
		"versions": map[string]string{"sonarr": compat.SonarrVersion, "radarr": compat.RadarrVersion}}, nil
}

func (a *API) getOldAppAPI(w http.ResponseWriter, r *http.Request) {
	v, err := a.oldAppAnswer(r)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) putOldAppAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled     *bool `json:"enabled"`
		DeleteFiles *bool `json:"delete_files"`
	}
	if !decode(w, r, &in, `send {"enabled": true} and/or {"delete_files": false}`) {
		return
	}
	if err := a.saveOldAppSwitches(r.Context(), actor(r), in.Enabled, in.DeleteFiles); err != nil {
		internal(w, err)
		return
	}
	a.getOldAppAPI(w, r)
}

// saveOldAppSwitches saves the old-app API's on/off and delete-files
// switches; a nil one is left as it is. Setup steps in this package (such
// as connecting Prowlarr) turn the API on through it.
func (a *API) saveOldAppSwitches(ctx context.Context, who domain.Actor, enabled, deleteFiles *bool) error {
	now := database.FormatTime(time.Now())
	by, _ := json.Marshal(who)
	return a.s.Store.Write(ctx, func(tx *sql.Tx) error {
		for key, v := range map[string]*bool{settingOldAppAPI: enabled, settingOldAppDelete: deleteFiles} {
			if v == nil {
				continue
			}
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				key, map[bool]string{true: "on", false: "off"}[*v], now, string(by), now, string(by))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// makeOldAppKey makes (or replaces) one side's key and shows it once. The
// key acts as the person who made it, so they must be allowed to manage
// the Library, and signed in with their password.
func (a *API) makeOldAppKey(w http.ResponseWriter, r *http.Request) {
	side := r.PathValue("side")
	if side != "sonarr" && side != "radarr" {
		fail(w, http.StatusNotFound, "the side is sonarr or radarr")
		return
	}
	if a.s.Auth == nil {
		fail(w, http.StatusConflict, "sign-in is off, so no key can be made")
		return
	}
	if !auth.RequireStrong(w, r) {
		return
	}
	// The new key can change the Library, so a HolyArrs API key making it
	// must hold the write scope too: a key never makes one that reaches more.
	if who, ok := auth.From(r.Context()); ok && !who.HasScope(auth.ScopeWrite) {
		fail(w, http.StatusForbidden, "this API key needs the write scope to make a key that changes the Library")
		return
	}
	if who, ok := auth.From(r.Context()); ok && !who.Has(auth.PermManageLibrary) {
		fail(w, http.StatusForbidden, "the key acts as you, and your account may not manage the Library")
		return
	}
	k, err := a.s.Auth.MakeCompatKey(r, side)
	if errors.Is(err, auth.ErrNoCompatSide) {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"side": side, "key": k, "path": webserver.URLBaseFrom(r.Context()) + "/" + side,
		"note": "This key is shown only now. Paste it into Seerr or Overseerr as the API key; an older key for this side no longer works."})
}

func (a *API) deleteOldAppKey(w http.ResponseWriter, r *http.Request) {
	side := r.PathValue("side")
	if a.s.Auth == nil || (side != "sonarr" && side != "radarr") {
		fail(w, http.StatusNotFound, "no such key")
		return
	}
	if !auth.RequireStrong(w, r) {
		return
	}
	ok, err := a.s.Auth.DeleteCompatKey(r, side)
	if err != nil {
		internal(w, err)
		return
	}
	if !ok {
		fail(w, http.StatusNotFound, "no such key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
