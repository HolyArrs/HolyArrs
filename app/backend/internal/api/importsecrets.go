package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importsecrets"
)

// importSecretsRoutes: the owner's secrets file, sent by `holyarrs
// import-secrets` while the app runs, and the move credentials it saves.
func (a *API) importSecretsRoutes(m *http.ServeMux) {
	if a.s.Secrets == nil {
		return
	}
	m.HandleFunc("POST /api/v1/settings/import-secrets", a.importSecrets)
	m.HandleFunc("GET /api/v1/migrate/saved-keys", a.savedMoveKeys)
	m.HandleFunc("DELETE /api/v1/migrate/saved-keys", a.forgetMoveKeys)
}

// importSecrets stores a parsed secrets file: {"dry_run": false, "file":
// {...}}. The answer names items and results only, never a value.
func (a *API) importSecrets(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, importsecrets.MaxSize*2))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, importsecrets.ErrTooBig.Error())
		return
	}
	var in struct {
		DryRun bool            `json:"dry_run"`
		File   json.RawMessage `json:"file"`
	}
	if err := json.Unmarshal(body, &in); err != nil || len(in.File) == 0 {
		fail(w, http.StatusBadRequest, `send {"dry_run": false, "file": {...the parsed secrets file...}}`)
		return
	}
	f, err := importsecrets.DecodeJSON(in.File)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// The running app is reached with this key; it is never stored here.
	f.HolyArrs.Address, f.HolyArrs.OwnerAPIKey = "", ""
	lines := importsecrets.Apply(r.Context(), importsecrets.Deps{Store: a.s.Store, Vault: a.s.Secrets, By: actor(r)}, f, in.DryRun)
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines, "ok": importsecrets.AllOK(lines)})
}

// savedMoveKeys lists the old apps with a saved key and their addresses;
// never a key.
func (a *API) savedMoveKeys(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	list, err := importsecrets.ListMoveCredentials(r.Context(), a.s.Store, a.s.Secrets)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": list})
}

// forgetMoveKeys deletes every saved old-app key (?kind= for one).
func (a *API) forgetMoveKeys(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	n, err := importsecrets.ForgetMoveCredential(r.Context(), a.s.Store, r.URL.Query().Get("kind"))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"forgotten": n})
}
