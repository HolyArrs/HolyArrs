package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/importsecrets"
	"github.com/CoderLuii/holyarrs/internal/migrate"
)

// readTimeout bounds one whole read of an old app.
var readTimeout = 5 * time.Minute

// migrateLimits are the readers' limits (tests make them small).
var migrateLimits migrate.Limits

func (a *API) migrateRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/migrate/probe", a.migrateProbe)
	m.HandleFunc("POST /api/v1/migrate/read", a.migrateRead)
	m.HandleFunc("GET /api/v1/migrate/snapshots", a.listSnapshots)
	m.HandleFunc("GET /api/v1/migrate/snapshots/{id}", a.getSnapshot)
	m.HandleFunc("DELETE /api/v1/migrate/snapshots/{id}", a.deleteSnapshot)
}

// MigrateSourceInput says which old app to read and how: an address with
// its key (qBittorrent: user name and password, or its API key), or a
// backup path on this server. Keys and passwords are used for this call
// only: never stored, logged or answered.
type MigrateSourceInput struct {
	Kind       migrate.Kind `json:"kind"`
	URL        string       `json:"url"`
	Key        string       `json:"key"`
	Username   string       `json:"username"`
	Password   string       `json:"password"`
	Path       string       `json:"path"`
	AllowLocal bool         `json:"allow_local_network"`
	// UseSavedKey reads with the key the secrets file saved for this app
	// (owner only); the address is the saved one unless url is given. A
	// successful read deletes the saved key.
	UseSavedKey bool `json:"use_saved_key"`
}

// migrateSource reads and checks the body; it answers and returns false
// on a bad one. saved says the read uses a saved key.
func (a *API) migrateSource(w http.ResponseWriter, r *http.Request) (src migrate.Source, savedID importsecrets.MoveCredentialVersion, ok bool) {
	var in MigrateSourceInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "the body must be JSON naming the app and an address or a backup path")
		return migrate.Source{}, importsecrets.MoveCredentialVersion{}, false
	}
	if !migrate.ValidKind(in.Kind) {
		fail(w, http.StatusBadRequest, "unknown app; pick one of radarr, sonarr, lidarr, prowlarr, bazarr, seerr, overseerr, qbittorrent, sabnzbd")
		return migrate.Source{}, importsecrets.MoveCredentialVersion{}, false
	}
	if in.UseSavedKey {
		// Fail closed: only a signed-in owner may use a saved key.
		if who, signed := auth.From(r.Context()); !signed || who.Role != auth.RoleOwner {
			fail(w, http.StatusForbidden, "only the owner may use a saved key")
			return migrate.Source{}, importsecrets.MoveCredentialVersion{}, false
		}
		c, id, has, err := importsecrets.ReadMoveCredentialVersion(r.Context(), a.s.Store, a.s.Secrets, string(in.Kind))
		if err != nil {
			internal(w, err)
			return migrate.Source{}, importsecrets.MoveCredentialVersion{}, false
		}
		if !has || in.Path != "" {
			fail(w, http.StatusBadRequest, "there is no saved key for this app; type its key")
			return migrate.Source{}, importsecrets.MoveCredentialVersion{}, false
		}
		// The key goes only to the address it was saved with, with only
		// the local-network leave it was saved with.
		if want := sameAddress(in.URL); in.URL != "" && (want == "" || want != sameAddress(c.Address)) {
			fail(w, http.StatusBadRequest, "a saved key is only sent to its saved address")
			return migrate.Source{}, importsecrets.MoveCredentialVersion{}, false
		}
		in.URL, in.Key, in.AllowLocal, savedID = c.Address, c.APIKey, c.AllowLocalNetwork, id
	}
	if (in.URL == "") == (in.Path == "") {
		fail(w, http.StatusBadRequest, migrate.ErrBadSource.Error())
		return migrate.Source{}, importsecrets.MoveCredentialVersion{}, false
	}
	if in.Path != "" && !filepath.IsAbs(in.Path) {
		fail(w, http.StatusBadRequest, "the backup path must be a full path on this server")
		return migrate.Source{}, importsecrets.MoveCredentialVersion{}, false
	}
	return migrate.Source{Kind: in.Kind, URL: in.URL, Key: in.Key, Username: in.Username, Password: in.Password,
		Path: in.Path, AllowLocal: in.AllowLocal, Limits: migrateLimits}, savedID, true
}

// migrateFail answers a reader's error in plain words.
func migrateFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, migrate.ErrUnreachable), errors.Is(err, migrate.ErrBadAnswer):
		fail(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, migrate.ErrSnapshotTooBig), errors.Is(err, migrate.ErrTooBig):
		fail(w, http.StatusRequestEntityTooLarge, migrate.ErrTooBig.Error())
	case errors.Is(err, migrate.ErrBackupBusy):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, migrate.ErrHeldKey):
		fail(w, http.StatusInternalServerError, err.Error())
	default:
		for _, e := range []error{migrate.ErrWrongApp, migrate.ErrBadKey, migrate.ErrNotBackup, migrate.ErrUnsafeBackup,
			migrate.ErrNotSQLite, migrate.ErrBadSource, migrate.ErrVersion} {
			if errors.Is(err, e) {
				fail(w, http.StatusUnprocessableEntity, e.Error())
				return
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			fail(w, http.StatusGatewayTimeout, "the read took too long")
			return
		}
		fail(w, http.StatusBadRequest, "the app could not be read: "+err.Error())
	}
}

func (a *API) migrateProbe(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	src, _, ok := a.migrateSource(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()
	info, err := migrate.Probe(ctx, src)
	if err != nil {
		migrateFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"app": info})
}

func (a *API) migrateRead(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	src, usedSaved, ok := a.migrateSource(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()
	snap, err := migrate.Read(ctx, src)
	if err != nil {
		migrateFail(w, err)
		return
	}
	var saved migrate.Saved
	err = a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		var err error
		saved, err = migrate.SaveTx(r.Context(), tx, actor(r).ID, snap)
		if err != nil {
			return err
		}
		if usedSaved.SecretID != 0 {
			return importsecrets.ForgetMoveCredentialTx(r.Context(), tx, string(src.Kind), usedSaved)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, importsecrets.ErrMoveCredentialChanged) {
			fail(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, migrate.ErrSnapshotTooBig) {
			migrateFail(w, err)
			return
		}
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"snapshot": saved, "needs_reentry": snap.NeedsReentry, "notes": snap.Notes})
}

func (a *API) listSnapshots(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	list, err := migrate.List(r.Context(), a.s.Store)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": list})
}

func snapshotID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusNotFound, migrate.ErrNoSnapshot.Error())
		return 0, false
	}
	return id, true
}

func (a *API) getSnapshot(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := snapshotID(w, r)
	if !ok {
		return
	}
	saved, snap, err := migrate.Get(r.Context(), a.s.Store, id)
	if errors.Is(err, migrate.ErrNoSnapshot) {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshot": saved, "data": snap})
}

func (a *API) deleteSnapshot(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := snapshotID(w, r)
	if !ok {
		return
	}
	if err := migrate.Delete(r.Context(), a.s.Store, id); err != nil {
		if errors.Is(err, migrate.ErrNoSnapshot) {
			fail(w, http.StatusNotFound, err.Error())
			return
		}
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sameAddress normalises an address for comparison: scheme and host in
// lower case, no trailing slash; "" when unreadable.
func sameAddress(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/") + "?" + u.RawQuery
}
