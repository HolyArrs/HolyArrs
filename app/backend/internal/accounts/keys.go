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
	// MCPGroups are an AI key's tick boxes; left out for other keys.
	MCPGroups []string `json:"mcp_groups,omitempty"`
}

// MCPGroups are the tick boxes of an AI key, in screen order, and
// MCPDefault what a key holds unless told otherwise. They are the groups of
// internal/ai/mcpkeys (its test keeps them the same).
var (
	MCPGroups  = []string{"see", "add", "control", "settings", "delete", "admin", "approve", "plugins"}
	MCPDefault = []string{"see", "add", "control", "settings"}
)

// defaultGroupsJSON is MCPDefault as stored.
var defaultGroupsJSON = func() string { b, _ := json.Marshal(MCPDefault); return string(b) }()

// keepGroups keeps the known names, in MCPGroups order, once each; ok is
// false when a name is unknown.
func keepGroups(names []string) (out []string, ok bool) {
	ok = true
	for _, n := range names {
		if !slices.Contains(MCPGroups, n) {
			ok = false
		}
	}
	out = []string{}
	for _, g := range MCPGroups {
		if slices.Contains(names, g) {
			out = append(out, g)
		}
	}
	return out, ok
}

// parseGroups reads a key's stored tick boxes, keeping known groups.
func parseGroups(s string) []string {
	var raw []string
	_ = json.Unmarshal([]byte(s), &raw)
	out, _ := keepGroups(raw)
	return out
}

// groupsBody checks the tick boxes sent for a key and returns them as
// stored; "" when none were sent.
func groupsBody(w http.ResponseWriter, in *[]string) (string, bool) {
	if in == nil {
		return "", true
	}
	set, ok := keepGroups(*in)
	if !ok {
		fail(w, http.StatusBadRequest, "mcp_groups are "+strings.Join(MCPGroups, ", "))
		return "", false
	}
	b, _ := json.Marshal(set)
	return string(b), true
}

func (a *Auth) checkKey(ctx context.Context, k string) (webserver.Caller, bool) {
	if !strings.HasPrefix(k, keyPrefix) {
		return webserver.Caller{}, false
	}
	var who webserver.Caller
	keyID, used, scope, groups, perms, err := keyBy(ctx, a.s.Read(), sum(k), &who)
	if err != nil {
		return webserver.Caller{}, false
	}
	a.fill(&who, perms)
	who.Scopes = parseScopes(scope)
	if slices.Contains(who.Scopes, webserver.ScopeMCP) {
		who.MCPGroups = parseGroups(groups)
	}
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

// errMixedMCP is the answer to a key asked for with mcp and another scope.
const errMixedMCP = "an AI key (mcp) holds no other scope"

// scopeProblem says why no key may be made with scope sc, or "". What the
// asker may hand out is checked by the caller.
func scopeProblem(sc string) string {
	if !slices.Contains(Scopes, sc) {
		return "scopes are " + strings.Join(Scopes, ", ") + " (all includes read, write and admin)"
	}
	if sc == webserver.ScopeCompat {
		return "make arr API keys in Settings > API (POST /api/v1/settings/old-app-api/keys/{side}), not here"
	}
	return ""
}

// mixesMCP reports whether scopes hold mcp next to another scope.
func mixesMCP(scopes []string) bool {
	return slices.Contains(scopes, webserver.ScopeMCP) && len(slices.Compact(slices.Sorted(slices.Values(scopes)))) > 1
}

func (a *Auth) makeKey(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Label     string    `json:"label"`
		Scopes    []string  `json:"scopes"`
		MCPGroups *[]string `json:"mcp_groups"`
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
		if msg := scopeProblem(sc); msg != "" {
			fail(w, http.StatusBadRequest, msg)
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
	if mixesMCP(body.Scopes) {
		fail(w, http.StatusBadRequest, errMixedMCP)
		return
	}
	body.Scopes = slices.Compact(slices.Sorted(slices.Values(body.Scopes)))
	isAI := slices.Contains(body.Scopes, webserver.ScopeMCP)
	if body.MCPGroups != nil && !isAI {
		fail(w, http.StatusBadRequest, "mcp_groups are only for an AI key (mcp)")
		return
	}
	groups, ok := groupsBody(w, body.MCPGroups)
	if !ok {
		return
	}
	scopeJSON, _ := json.Marshal(body.Scopes)
	k := keyPrefix + token(32)
	now := database.FormatTime(a.now())
	actor, _ := json.Marshal(webserver.Actor(r.Context()))
	var id int64
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		var err error
		id, err = insertKey(r.Context(), tx, now, string(actor), who.UserID, body.Label, string(scopeJSON), groups, sum(k), k[len(k)-4:])
		return err
	})
	if err != nil {
		log.Printf("auth: key: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	a.event(r, webserver.Actor(r.Context()), "api_key_created", "API key "+body.Label+" created", map[string]any{"key_id": id, "label": body.Label, "scopes": body.Scopes})
	out := map[string]any{"id": id, "label": body.Label, "scopes": body.Scopes, "key": k,
		"note": "This key is shown only now. Send it in the " + APIKeyHeader + " header."}
	if isAI {
		if groups == "" {
			groups = defaultGroupsJSON
		}
		out["mcp_groups"] = parseGroups(groups)
	}
	writeJSON(w, http.StatusCreated, out)
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
		var scope, groups string
		if err := rows.Scan(&k.ID, &k.Label, &k.Hint, &k.CreatedAt, &used, &scope, &groups); err != nil {
			fail(w, http.StatusInternalServerError, "something went wrong; see the log")
			return
		}
		k.Hint = "…" + k.Hint
		k.Scopes = parseScopes(scope)
		if slices.Contains(k.Scopes, webserver.ScopeMCP) {
			k.MCPGroups = parseGroups(groups)
		}
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
		envHash, err := envKeyHash(r.Context(), tx, id)
		if err != nil {
			return err
		}
		n, err = deleteKey(r.Context(), tx, id, who.Role == webserver.RoleOwner, who.UserID)
		if err != nil || n == 0 || envHash == "" {
			return err
		}
		// A person deleting the file's key may know it leaked: the next
		// start must not make it again.
		return noteRevoked(r.Context(), tx, envHash, webserver.Actor(r.Context()), a.now())
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
