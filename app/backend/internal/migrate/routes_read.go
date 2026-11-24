package migrate

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

func (h *Handlers) readRoutes(m *webserver.Router) {
	m.Handle("POST /api/v1/migrate/probe", webserver.NeedUsers, http.HandlerFunc(h.probe))
	m.Handle("POST /api/v1/migrate/read", webserver.NeedUsers, http.HandlerFunc(h.read))
	m.Handle("POST /api/v1/migrate/preview", webserver.NeedUsers, http.HandlerFunc(h.preview))
	m.Handle("POST /api/v1/migrate/find", webserver.NeedOwner, http.HandlerFunc(h.find))
	m.Handle("POST /api/v1/migrate/linked", webserver.NeedOwner, http.HandlerFunc(h.linked))
	m.Handle("POST /api/v1/migrate/folders", webserver.NeedOwner, http.HandlerFunc(h.browseFolders))
	m.Handle("POST /api/v1/migrate/key-lookup", webserver.NeedOwner, http.HandlerFunc(h.keyLookup))
	m.Handle("GET /api/v1/migrate/snapshots", webserver.NeedUsers, http.HandlerFunc(h.listSnapshots))
	m.Handle("GET /api/v1/migrate/snapshots/{id}", webserver.NeedUsers, http.HandlerFunc(h.getSnapshot))
	m.Handle("DELETE /api/v1/migrate/snapshots/{id}", webserver.NeedUsers, http.HandlerFunc(h.deleteSnapshot))
}

// MigrateSourceInput says which old app to read and how: an address with
// its key (qBittorrent: user name and password, or its API key), or a
// backup path on this server. Keys and passwords are never logged or
// answered; after a successful read the owner's typed key is kept in the
// vault for "Read again" until a move finishes.
type MigrateSourceInput struct {
	Kind       Kind   `json:"kind"`
	URL        string `json:"url"`
	Key        string `json:"key"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Path       string `json:"path"`
	AllowLocal bool   `json:"allow_local_network"`
	// UseSavedKey reads with the key the secrets file saved for this app
	// or a past read kept for this app (owner only): the one kept for url,
	// or the newest when url is empty. The saved key stays after the read.
	UseSavedKey bool `json:"use_saved_key"`
	// Add names the addresses of linked apps to read in one go (only
	// /migrate/linked).
	Add []string `json:"add,omitempty"`
	// SkipLinked names addresses already read: a Seerr or Overseerr read
	// does not read its Sonarr or Radarr at those again.
	SkipLinked []string `json:"skip_linked,omitempty"`
	// LinkedFrom reads a Sonarr or Radarr at a new address with the key the
	// Seerr or Overseerr read before keeps for it (owner only; the key never
	// reaches the browser).
	LinkedFrom *LinkedFrom `json:"linked_from,omitempty"`
}

// source reads and checks the body; it answers and returns false
// on a bad one. saved says the read uses a saved key.
func (h *Handlers) source(w http.ResponseWriter, r *http.Request) (src Source, savedID secretsfile.MoveCredentialVersion, ok bool) {
	var in MigrateSourceInput
	return h.sourceFrom(w, r, &in)
}

// sourceFrom is source, keeping the decoded body in in.
func (h *Handlers) sourceFrom(w http.ResponseWriter, r *http.Request, in *MigrateSourceInput) (src Source, savedID secretsfile.MoveCredentialVersion, ok bool) {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(in); err != nil {
		webserver.Fail(w, http.StatusBadRequest, "the body must be JSON naming the app and an address or a backup path")
		return Source{}, secretsfile.MoveCredentialVersion{}, false
	}
	if !ValidKind(in.Kind) {
		webserver.Fail(w, http.StatusBadRequest, "unknown app; pick one of radarr, sonarr, lidarr, prowlarr, bazarr, seerr, overseerr, qbittorrent, sabnzbd")
		return Source{}, secretsfile.MoveCredentialVersion{}, false
	}
	if in.UseSavedKey {
		// Fail closed: only a signed-in owner may use a saved key.
		if who, signed := webserver.From(r.Context()); !signed || who.Role != webserver.RoleOwner {
			webserver.Fail(w, http.StatusForbidden, "only the owner may use a saved key")
			return Source{}, secretsfile.MoveCredentialVersion{}, false
		}
		// The key goes only to the address it was saved with, with only
		// the local-network leave it was saved with.
		if in.URL != "" && sameAddress(in.URL) == "" {
			webserver.Fail(w, http.StatusBadRequest, "a saved key is only sent to its saved address")
			return Source{}, secretsfile.MoveCredentialVersion{}, false
		}
		c, id, has, err := secretsfile.ReadMoveCredentialAt(r.Context(), h.Store, h.Secrets, string(in.Kind), in.URL)
		if err != nil {
			webserver.Internal(w, "migrate", err)
			return Source{}, secretsfile.MoveCredentialVersion{}, false
		}
		if !has || in.Path != "" {
			webserver.Fail(w, http.StatusBadRequest, "there is no saved key for this app at this address; type its key")
			return Source{}, secretsfile.MoveCredentialVersion{}, false
		}
		in.URL, in.Key, in.Username, in.Password, in.AllowLocal, savedID = c.Address, c.APIKey, c.Username, c.Password, c.AllowLocalNetwork, id
	}
	if in.LinkedFrom != nil && in.Key == "" && in.Path == "" && !in.UseSavedKey {
		if !h.linkedKey(w, r, in) {
			return Source{}, secretsfile.MoveCredentialVersion{}, false
		}
	}
	if (in.URL == "") == (in.Path == "") {
		webserver.Fail(w, http.StatusBadRequest, ErrBadSource.Error())
		return Source{}, secretsfile.MoveCredentialVersion{}, false
	}
	if in.Path != "" && !filepath.IsAbs(in.Path) {
		webserver.Fail(w, http.StatusBadRequest, "the backup path must be a full path on this server")
		return Source{}, secretsfile.MoveCredentialVersion{}, false
	}
	return Source{Kind: in.Kind, URL: in.URL, Key: in.Key, Username: in.Username, Password: in.Password,
		Path: in.Path, AllowLocal: in.AllowLocal, Limits: h.Limits}, savedID, true
}

// fail answers a reader's error in plain words.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnreachable), errors.Is(err, ErrBadAnswer):
		webserver.Fail(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, ErrSnapshotTooBig), errors.Is(err, ErrTooBig):
		webserver.Fail(w, http.StatusRequestEntityTooLarge, ErrTooBig.Error())
	case errors.Is(err, ErrBackupBusy):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrHeldKey):
		webserver.Fail(w, http.StatusInternalServerError, err.Error())
	default:
		for _, e := range []error{ErrWrongApp, ErrBadKey, ErrNotBackup, ErrUnsafeBackup,
			ErrNotSQLite, ErrBadSource, ErrVersion} {
			if errors.Is(err, e) {
				webserver.Fail(w, http.StatusUnprocessableEntity, e.Error())
				return
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			webserver.Fail(w, http.StatusGatewayTimeout, "the read took too long")
			return
		}
		webserver.Fail(w, http.StatusBadRequest, "the app could not be read: "+err.Error())
	}
}

func (h *Handlers) probe(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	src, _, ok := h.source(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()
	info, err := Probe(ctx, src)
	if err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"app": info})
}

func (h *Handlers) read(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var in MigrateSourceInput
	src, usedSaved, ok := h.sourceFrom(w, r, &in)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()
	movesDone := h.movesDone(r.Context())
	snap, err := Read(ctx, src)
	if err != nil {
		fail(w, err)
		return
	}
	if src.Path == "" {
		snap.Address = InstanceAddress(src.URL)
	}
	// The same app read before at another address (same key) is replaced, so it counts once.
	older := h.sameAppReads(r.Context(), src, snap.Address)
	var saved Saved
	err = h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		var err error
		saved, err = SaveTx(r.Context(), tx, actor(r).ID, snap)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrSnapshotTooBig) {
			fail(w, err)
			return
		}
		webserver.Internal(w, "migrate", err)
		return
	}
	// A move that finished while this read ran has forgotten every key: this
	// read's key stays forgotten too.
	if h.movesDone(r.Context()) == movesDone {
		h.keepMoveKey(r, src, usedSaved)
	}
	replaced := h.dropSameApp(r.Context(), older)
	linked, unread, skipped := h.readSeerrArrs(r, src, movesDone, in.SkipLinked)
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"snapshot": saved, "replaced": replaced, "counts": previewCounts(snap), "needs_reentry": snap.NeedsReentry, "notes": snap.Notes, "linked": linked, "unread": unread, "skipped": skipped})
}

// LinkedRead is a Sonarr or Radarr read along with the Overseerr or Seerr
// that keeps it.
type LinkedRead struct {
	Kind     Kind  `json:"kind"`
	Snapshot Saved `json:"snapshot"`
	// Replaced names older reads of the same app at other addresses (same
	// key), now forgotten, so the app counts once.
	Replaced []int64 `json:"replaced,omitempty"`
}

// LinkedUnread is a Sonarr or Radarr an Overseerr or Seerr keeps that
// HolyArrs could not read at the address it keeps (often a Docker name).
type LinkedUnread struct {
	Kind    Kind   `json:"kind"`
	Address string `json:"address"`
}

// readSeerrArrs reads the Sonarr and Radarr servers an Overseerr or Seerr
// keeps, with the keys it keeps, and keeps each key in the vault as a typed
// read does. Addresses in skip are not read again; they come back as skipped
// so the page can still count them. One that does not answer is listed as
// unread: the Overseerr read counts. The same app kept under two addresses
// (same kind and API key) is read once.
func (h *Handlers) readSeerrArrs(r *http.Request, src Source, movesDone int, skip []string) ([]LinkedRead, []LinkedUnread, []LinkedUnread) {
	out, unread, skipped := []LinkedRead{}, []LinkedUnread{}, []LinkedUnread{}
	if src.URL == "" {
		return out, unread, skipped
	}
	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()
	arrs, err := SeerrArrs(ctx, src)
	if err != nil {
		return out, unread, skipped
	}
	done := map[string]bool{}
	for _, a := range skip {
		done[InstanceAddress(a)] = true
	}
	// An app's API key is its identity here: two addresses with the same key are one app.
	same := func(c Source) string { return string(c.Kind) + " " + c.Key }
	seen := map[string]bool{}
	for _, child := range arrs {
		if done[InstanceAddress(child.URL)] {
			skipped = append(skipped, LinkedUnread{Kind: child.Kind, Address: InstanceAddress(child.URL)})
			if child.Key != "" {
				seen[same(child)] = true
			}
		}
	}
	var failed []Source
	for _, child := range arrs {
		if done[InstanceAddress(child.URL)] || (child.Key != "" && seen[same(child)]) {
			continue
		}
		snap, err := Read(ctx, child)
		if err != nil {
			failed = append(failed, child)
			continue
		}
		if child.Key != "" {
			seen[same(child)] = true
		}
		snap.Address = InstanceAddress(child.URL)
		older := h.sameAppReads(r.Context(), child, snap.Address)
		var saved Saved
		if err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
			var err error
			saved, err = SaveTx(r.Context(), tx, actor(r).ID, snap)
			return err
		}); err != nil {
			continue
		}
		if h.Secrets != nil && h.movesDone(r.Context()) == movesDone {
			h.keepMoveKey(r, child, secretsfile.MoveCredentialVersion{})
		}
		out = append(out, LinkedRead{Kind: child.Kind, Snapshot: saved, Replaced: h.dropSameApp(r.Context(), older)})
	}
	// An address that did not answer for an app read at another address needs no look.
	for _, child := range failed {
		if child.Key == "" || !seen[same(child)] {
			unread = append(unread, LinkedUnread{Kind: child.Kind, Address: InstanceAddress(child.URL)})
		}
	}
	return out, unread, skipped
}

// keepMoveKey keeps the owner's typed key (or qBittorrent sign-in) after a
// good read, replacing any older one for the app, so "Read again" needs no
// typing. A read with the saved key leaves it as it is. Keeping is best
// effort: the read already counts.
func (h *Handlers) keepMoveKey(r *http.Request, src Source, usedSaved secretsfile.MoveCredentialVersion) {
	if usedSaved.SecretID != 0 || src.Path != "" || src.URL == "" {
		return
	}
	if who, signed := webserver.From(r.Context()); !signed || who.Role != webserver.RoleOwner {
		return
	}
	c := secretsfile.MoveCredential{Address: src.URL, APIKey: src.Key, Username: src.Username, Password: src.Password, AllowLocalNetwork: src.AllowLocal}
	_ = secretsfile.SaveMoveCredential(context.WithoutCancel(r.Context()), h.Store, h.Secrets, string(src.Kind), c, actor(r))
}

// movesDone counts the finished moves (-2 when it can't be read).
func (h *Handlers) movesDone(ctx context.Context) int {
	n, err := CountDoneMoves(ctx, h.Store.Read())
	if err != nil {
		return -2
	}
	return n
}

func (h *Handlers) listSnapshots(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	list, err := List(r.Context(), h.Store)
	if err != nil {
		webserver.Internal(w, "migrate", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"snapshots": list})
}

func snapshotID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, ErrNoSnapshot.Error())
		return 0, false
	}
	return id, true
}

func (h *Handlers) getSnapshot(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := snapshotID(w, r)
	if !ok {
		return
	}
	saved, snap, err := Get(r.Context(), h.Store, id)
	if errors.Is(err, ErrNoSnapshot) {
		webserver.Fail(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "migrate", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"snapshot": saved, "data": snap})
}

func (h *Handlers) deleteSnapshot(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := snapshotID(w, r)
	if !ok {
		return
	}
	kind, address, _ := SnapshotKind(r.Context(), h.Store.Read(), id)
	if err := Delete(r.Context(), h.Store, id); err != nil {
		if errors.Is(err, ErrNoSnapshot) {
			webserver.Fail(w, http.StatusNotFound, err.Error())
			return
		}
		webserver.Internal(w, "migrate", err)
		return
	}
	// Forgetting a read also forgets the key kept for that app (a read
	// kept before addresses were forgets every key of its kind).
	if kind != "" {
		var err error
		if address != "" {
			_, err = secretsfile.ForgetMoveCredentialAt(r.Context(), h.Store, h.Secrets, kind, address)
		} else {
			_, err = secretsfile.ForgetMoveCredential(r.Context(), h.Store, kind)
		}
		if err != nil {
			webserver.Internal(w, "migrate", err)
			return
		}
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

// InstanceAddress is the plain base address a read came from: scheme and
// host in lower case, no user, query, fragment or trailing slash; it
// names one copy of an old app.
func InstanceAddress(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	u.User, u.RawQuery, u.ForceQuery, u.Fragment = nil, "", false, ""
	return strings.TrimRight(u.String(), "/")
}
