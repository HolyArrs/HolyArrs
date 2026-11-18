package accounts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// editKey changes an AI key's tick boxes. Only the owner and admins may,
// and only signed in: a key never edits a key. Like delete, an admin reaches
// only their own keys; the owner reaches every key. Scopes stay fixed. The key's
// next call reads the new tick boxes, since every call reads them afresh.
func (a *Auth) editKey(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	who, _ := webserver.From(r.Context())
	if who.Via == "api_key" || (who.Role != webserver.RoleOwner && who.Role != webserver.RoleAdmin) {
		fail(w, http.StatusForbidden, "only the owner and admins may change a key, signed in")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such key")
		return
	}
	var body struct {
		MCPGroups *[]string `json:"mcp_groups"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || body.MCPGroups == nil {
		fail(w, http.StatusBadRequest, `send {"mcp_groups": [...]}; a key's scopes cannot change`)
		return
	}
	groups, ok := groupsBody(w, body.MCPGroups)
	if !ok {
		return
	}
	now := database.FormatTime(a.now())
	actor, _ := json.Marshal(webserver.Actor(r.Context()))
	var label, before string
	notAI := false
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		var scope string
		var err error
		label, scope, before, err = keyForEdit(r.Context(), tx, id, who.Role == webserver.RoleOwner, who.UserID)
		if err != nil {
			return err
		}
		if !slices.Contains(parseScopes(scope), webserver.ScopeMCP) {
			notAI = true
			return nil
		}
		return setKeyGroups(r.Context(), tx, id, groups, now, string(actor))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "no such key")
		return
	case err != nil:
		log.Printf("auth: edit key: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	case notAI:
		fail(w, http.StatusBadRequest, "mcp_groups are only for an AI key (mcp)")
		return
	}
	set := parseGroups(groups)
	a.event(r, webserver.Actor(r.Context()), "api_key_changed", "API key "+label+" changed", map[string]any{"key_id": id, "label": label, "mcp_groups_before": parseGroups(before), "mcp_groups": set})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "label": label, "mcp_groups": set})
}
