package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// KeyView is an API key without its value.
type KeyView struct {
	ID         int64    `json:"id"`
	Label      string   `json:"label"`
	Hint       string   `json:"hint"`
	CreatedAt  string   `json:"created_at"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	Scopes     []string `json:"scopes"`
}

func (a *Auth) checkKey(ctx context.Context, k string) (webserver.Caller, bool) {
	if !strings.HasPrefix(k, keyPrefix) {
		return webserver.Caller{}, false
	}
	var who webserver.Caller
	keyID, used, scope, perms, err := keyBy(ctx, a.s.Read(), sum(k), &who)
	if err != nil {
		return webserver.Caller{}, false
	}
	a.fill(&who, perms)
	who.Scopes = parseScopes(scope)
	now := a.now()
	if !used.Valid || now.Sub(parseTime(used.String)) > touchEvery {
		_ = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
			return touchKey(ctx, tx, database.FormatTime(now), keyID)
		})
	}
	who.Via = "api_key"
	who.KeyID = keyID
	return who, true
}

func (a *Auth) makeKey(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Label  string   `json:"label"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"label": "...", "scopes": ["all"]}`)
		return
	}
	body.Label = strings.TrimSpace(body.Label)
	if body.Label == "" || len(body.Label) > 64 {
		fail(w, http.StatusBadRequest, "give the key a label of 1 to 64 characters")
		return
	}
	who, _ := webserver.From(r.Context())
	if len(body.Scopes) == 0 {
		body.Scopes = []string{webserver.ScopeAll}
	}
	for _, sc := range body.Scopes {
		if !slices.Contains(Scopes, sc) {
			fail(w, http.StatusBadRequest, "scopes are "+strings.Join(Scopes, ", ")+" (all includes read, write and admin)")
			return
		}
		if sc == webserver.ScopeCompat {
			fail(w, http.StatusBadRequest, "make arr API keys in Settings > API (POST /api/v1/settings/old-app-api/keys/{side}), not here")
			return
		}
		if !who.HasScope(sc) {
			fail(w, http.StatusForbidden, "a key cannot make a key with a scope it does not hold")
			return
		}
		if sc == webserver.ScopeControl && !who.Has(webserver.NeedUsers) {
			fail(w, http.StatusForbidden, "only the owner and admins may make a key with the control scope")
			return
		}
	}
	if slices.Contains(body.Scopes, webserver.ScopeMCP) && len(slices.Compact(slices.Sorted(slices.Values(body.Scopes)))) > 1 {
		fail(w, http.StatusBadRequest, "an AI key (mcp) holds no other scope")
		return
	}
	body.Scopes = slices.Compact(slices.Sorted(slices.Values(body.Scopes)))
	scopeJSON, _ := json.Marshal(body.Scopes)
	k := keyPrefix + token(32)
	now := database.FormatTime(a.now())
	actor, _ := json.Marshal(webserver.Actor(r.Context()))
	var id int64
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		var err error
		id, err = insertKey(r.Context(), tx, now, string(actor), who.UserID, body.Label, string(scopeJSON), sum(k), k[len(k)-4:])
		return err
	})
	if err != nil {
		log.Printf("auth: key: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	a.event(r, webserver.Actor(r.Context()), "api_key_created", "API key "+body.Label+" created", map[string]any{"key_id": id, "label": body.Label, "scopes": body.Scopes})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "label": body.Label, "scopes": body.Scopes, "key": k,
		"note": "This key is shown only now. Send it in the " + APIKeyHeader + " header."})
}

// parseScopes reads a key's stored scope list, keeping known scopes.
func parseScopes(s string) []string {
	var raw []string
	_ = json.Unmarshal([]byte(s), &raw)
	out := []string{}
	for _, sc := range raw {
		if slices.Contains(Scopes, sc) && !slices.Contains(out, sc) {
			out = append(out, sc)
		}
	}
	return out
}

func (a *Auth) listKeys(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	// The owner sees every key; others see their own.
	who, _ := webserver.From(r.Context())
	rows, err := listKeyRows(r.Context(), a.s.Read(), who.Role == webserver.RoleOwner, who.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	defer rows.Close()
	out := []KeyView{}
	for rows.Next() {
		var k KeyView
		var used sql.NullString
		var scope string
		if err := rows.Scan(&k.ID, &k.Label, &k.Hint, &k.CreatedAt, &used, &scope); err != nil {
			fail(w, http.StatusInternalServerError, "something went wrong; see the log")
			return
		}
		k.Hint = "…" + k.Hint
		k.Scopes = parseScopes(scope)
		if used.Valid {
			k.LastUsedAt = &used.String
		}
		out = append(out, k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (a *Auth) deleteKey(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such key")
		return
	}
	var n int64
	var label string
	who, _ := webserver.From(r.Context())
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		label = keyLabel(r.Context(), tx, id)
		var err error
		n, err = deleteKey(r.Context(), tx, id, who.Role == webserver.RoleOwner, who.UserID)
		return err
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	if n == 0 {
		fail(w, http.StatusNotFound, "no such key")
		return
	}
	a.event(r, webserver.Actor(r.Context()), "api_key_deleted", "API key "+label+" deleted", map[string]any{"key_id": id, "label": label})
	w.WriteHeader(http.StatusNoContent)
}
