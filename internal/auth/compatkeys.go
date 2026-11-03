package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/store"
)

// The old-app API (decision 0008): /sonarr/ and /radarr/ answer apps such
// as Seerr and Overseerr as Sonarr and Radarr would. Those apps send their
// key in the address, so these keys are their own kind: one per side, kept
// apart from HolyArrs API keys (hak_), never accepted anywhere else, and
// masked in the log like every apikey= value.

// CompatSides are the two sides, in the order screens show them.
var CompatSides = []string{"sonarr", "radarr"}

// ErrNoCompatSide says the side is not sonarr or radarr.
var ErrNoCompatSide = errors.New("the side is sonarr or radarr")

// CompatSide names the side a path is under ("sonarr" for /sonarr/...),
// or "" when it is under neither.
func CompatSide(path string) string {
	for _, s := range CompatSides {
		if strings.HasPrefix(path, "/"+s+"/") {
			return s
		}
	}
	return ""
}

// compatKey reads the key the way those apps send it. Only the first place
// found counts, as in the old apps: the apikey query value (any case of
// the name), then the X-Api-Key header, then Authorization with or
// without "Bearer ". A wrong value in an earlier place is not saved by a
// right one in a later place.
func compatKey(r *http.Request) (string, bool) {
	for name, vs := range r.URL.Query() {
		if strings.EqualFold(name, "apikey") {
			if len(vs) == 0 {
				return "", true
			}
			return vs[0], true
		}
	}
	if vs, ok := r.Header[http.CanonicalHeaderKey(APIKeyHeader)]; ok && len(vs) > 0 {
		return vs[0], true
	}
	if v := r.Header.Get("Authorization"); v != "" {
		return strings.TrimSpace(strings.TrimPrefix(v, "Bearer ")), true
	}
	return "", false
}

// compatLink matches the page link Seerr shows admins to open a title in
// the old app (/sonarr/series/<id>, /radarr/movie/<id>, no API path). A
// browser follows it with no key, so it goes to the HolyArrs title page.
func compatLink(r *http.Request) (string, bool) {
	if r.Method != http.MethodGet {
		return "", false
	}
	for _, p := range []string{"/sonarr/series/", "/radarr/movie/"} {
		if id, ok := strings.CutPrefix(r.URL.Path, p); ok && id != "" && strings.Trim(id, "0123456789") == "" {
			return id, true
		}
	}
	return "", false
}

// checkCompatKey finds the person behind a side's key. The key works only
// on its own side, and only while its maker's account is active.
func (a *Auth) checkCompatKey(ctx context.Context, side, k string) (Who, bool) {
	if len(k) != 32 {
		return Who{}, false
	}
	var who Who
	var used sql.NullString
	var perms string
	err := a.s.Read().QueryRowContext(ctx, `SELECT k.last_used_at, `+userCols+` FROM compat_keys k JOIN users u ON u.id = k.owner_user_id
		WHERE k.side = ? AND k.hash = ? AND u.state = 'active'`, side, sum(k)).Scan(&used, &who.UserID, &who.Name, &who.Role, &perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry)
	if err != nil {
		return Who{}, false
	}
	a.fill(&who, perms)
	// Seerr calls every minute: the last-used time is written at most once
	// an hour, like API keys.
	now := a.now()
	if !used.Valid || now.Sub(parseTime(used.String)) > touchEvery {
		_ = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE compat_keys SET last_used_at = ? WHERE side = ?", store.FormatTime(now), side)
			return err
		})
	}
	who.Via = "api_key"
	who.Scopes = []string{ScopeCompat}
	who.KeyLabel = "old-app API (" + side + ")"
	return who, true
}

// CompatKeyView is one side's key without its value.
type CompatKeyView struct {
	Side       string  `json:"side"`
	Hint       string  `json:"hint"`
	Owner      string  `json:"owner"`
	OwnerOK    bool    `json:"owner_active"` // false: the maker's account is off, so the key is refused
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

// CompatKeys lists the sides that have a key.
func (a *Auth) CompatKeys(ctx context.Context) ([]CompatKeyView, error) {
	rows, err := a.s.Read().QueryContext(ctx, `SELECT k.side, k.hint, COALESCE(u.name, ''), COALESCE(u.state = 'active', 0), k.created_at, k.last_used_at
		FROM compat_keys k LEFT JOIN users u ON u.id = k.owner_user_id ORDER BY k.side DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CompatKeyView{}
	for rows.Next() {
		var v CompatKeyView
		var used sql.NullString
		if err := rows.Scan(&v.Side, &v.Hint, &v.Owner, &v.OwnerOK, &v.CreatedAt, &used); err != nil {
			return nil, err
		}
		v.Hint = "…" + v.Hint
		if used.Valid {
			v.LastUsedAt = &used.String
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// MakeCompatKey makes a new key for side, owned by the person asking, and
// returns it; this is the only time it is shown. An older key of that side
// stops working at once. The key is 32 lowercase hex characters, the form
// those apps expect.
func (a *Auth) MakeCompatKey(r *http.Request, side string) (string, error) {
	if side != "sonarr" && side != "radarr" {
		return "", ErrNoCompatSide
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	k := hex.EncodeToString(b)
	who, _ := From(r.Context())
	by, _ := json.Marshal(Actor(r.Context()))
	now := store.FormatTime(a.now())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO compat_keys(side, created_at, created_by, owner_user_id, hash, hint) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (side) DO UPDATE SET created_at = excluded.created_at, created_by = excluded.created_by, owner_user_id = excluded.owner_user_id,
			last_used_at = NULL, hash = excluded.hash, hint = excluded.hint`, side, now, string(by), who.UserID, sum(k), k[len(k)-4:])
		return err
	})
	if err != nil {
		return "", err
	}
	a.event(r, Actor(r.Context()), "api_key_created", "Old-app API key ("+side+") made", map[string]any{"old_app_side": side})
	return k, nil
}

// DeleteCompatKey removes side's key; false when it had none.
func (a *Auth) DeleteCompatKey(r *http.Request, side string) (bool, error) {
	var n int64
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec("DELETE FROM compat_keys WHERE side = ?", side)
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	if err != nil || n == 0 {
		return false, err
	}
	a.event(r, Actor(r.Context()), "api_key_deleted", "Old-app API key ("+side+") removed", map[string]any{"old_app_side": side})
	return true, nil
}
