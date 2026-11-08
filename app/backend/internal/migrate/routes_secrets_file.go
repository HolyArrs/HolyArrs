package migrate

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// secretsFileRoutes serves the owner's secrets file, sent by `holyarrs
// import-secrets` while the app runs, and the move credentials it saves.
func (h *Handlers) secretsFileRoutes(m *webserver.Router) {
	m.Handle("POST /api/v1/settings/import-secrets", webserver.NeedOwner, http.HandlerFunc(h.importSecrets))
	m.Handle("GET /api/v1/migrate/saved-keys", webserver.NeedOwner, http.HandlerFunc(h.savedMoveKeys))
	m.Handle("DELETE /api/v1/migrate/saved-keys", webserver.NeedOwner, http.HandlerFunc(h.forgetMoveKeys))
}

// importSecrets stores a parsed secrets file: {"dry_run": false, "file":
// {...}}. The answer names items and results only, never a value.
func (h *Handlers) importSecrets(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, secretsfile.MaxSize*2))
	if err != nil {
		webserver.Fail(w, http.StatusRequestEntityTooLarge, secretsfile.ErrTooBig.Error())
		return
	}
	var in struct {
		DryRun bool            `json:"dry_run"`
		File   json.RawMessage `json:"file"`
	}
	if err := json.Unmarshal(body, &in); err != nil || len(in.File) == 0 {
		webserver.Fail(w, http.StatusBadRequest, `send {"dry_run": false, "file": {...the parsed secrets file...}}`)
		return
	}
	f, err := secretsfile.DecodeJSON(in.File)
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// The running app is reached with this key; it is never stored here.
	f.HolyArrs.Address, f.HolyArrs.OwnerAPIKey = "", ""
	lines := secretsfile.Apply(r.Context(), secretsfile.Deps{Store: h.Store, Vault: h.Secrets, By: actor(r)}, f, in.DryRun)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"lines": lines, "ok": secretsfile.AllOK(lines)})
}

// savedMoveKeys lists the old apps with a saved key and their addresses;
// never a key.
func (h *Handlers) savedMoveKeys(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	list, err := secretsfile.ListMoveCredentials(r.Context(), h.Store, h.Secrets)
	if err != nil {
		webserver.Internal(w, "migrate", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"saved": list})
}

// forgetMoveKeys deletes every saved arr key (?kind= for one).
func (h *Handlers) forgetMoveKeys(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	n, err := secretsfile.ForgetMoveCredential(r.Context(), h.Store, r.URL.Query().Get("kind"))
	if err != nil {
		webserver.Internal(w, "migrate", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"forgotten": n})
}
