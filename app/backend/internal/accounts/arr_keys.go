package accounts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// The arr API: /sonarr/ and /radarr/ answer apps such
// as Seerr and Overseerr as Sonarr and Radarr would. Those apps send their
// key in the address, so these keys are their own kind: one per side, kept
// apart from HolyArrs API keys (hak_), never accepted anywhere else, and
// masked in the log like every apikey= value.

// ArrSides are the two sides, in the order screens show them.
var ArrSides = []string{"sonarr", "radarr"}

// ErrNoArrSide says the side is not sonarr or radarr.
var ErrNoArrSide = errors.New("the side is sonarr or radarr")

// ArrSide names the side a path is under ("sonarr" for /sonarr/...),
// or "" when it is under neither.
func ArrSide(path string) string {
	for _, s := range ArrSides {
		if strings.HasPrefix(path, "/"+s+"/") {
			return s
		}
	}
	return ""
}

// arrKey reads the key the way those apps send it. Only the first place
// found counts, as in the old apps: the apikey query value (any case of
// the name), then the X-Api-Key header, then Authorization with or
// without "Bearer ". A wrong value in an earlier place is not saved by a
// right one in a later place.
func arrKey(r *http.Request) (string, bool) {
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

// arrLink matches the page link Seerr shows admins to open a title in
// the old app (/sonarr/series/<id>, /radarr/movie/<id>, no API path). A
// browser follows it with no key, so it goes to the HolyArrs title page:
// a show's slug is its Library id, a movie's is its TMDB id.
func arrLink(r *http.Request) (string, bool) {
	if r.Method != http.MethodGet {
		return "", false
	}
	for p, to := range map[string]string{"/sonarr/series/": "/library/", "/radarr/movie/": "/title/movie/"} {
		if id, ok := strings.CutPrefix(r.URL.Path, p); ok && id != "" && strings.Trim(id, "0123456789") == "" {
			return to + id, true
		}
	}
	return "", false
}

// checkArrKey finds the person behind a side's key. The key works only
// on its own side, and only while its maker's account is active.
func (a *Auth) checkArrKey(ctx context.Context, side, k string) (webserver.Caller, bool) {
	if len(k) != 32 {
		return webserver.Caller{}, false
	}
	who, used, perms, err := a.arrKeyOwner(ctx, side, sum(k))
	if err != nil {
		return webserver.Caller{}, false
	}
	a.fill(&who, perms)
	// Seerr calls every minute: the last-used time is written at most once
	// an hour, like API keys.
	now := a.now()
	if !used.Valid || now.Sub(parseTime(used.String)) > touchEvery {
		_ = a.touchArrKey(ctx, side, now)
	}
	who.Via = "api_key"
	who.Scopes = []string{webserver.ScopeCompat}
	who.KeyLabel = "arr API (" + side + ")"
	return who, true
}

// ArrKeyView is one side's key without its value.
type ArrKeyView struct {
	Side       string  `json:"side"`
	Hint       string  `json:"hint"`
	Owner      string  `json:"owner"`
	OwnerOK    bool    `json:"owner_active"` // false: the maker's account is off, so the key is refused
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

// ArrKeys lists the sides that have a key.
func (a *Auth) ArrKeys(ctx context.Context) ([]ArrKeyView, error) {
	return a.listArrKeys(ctx)
}

// MakeArrKey makes a new key for side, owned by the person asking, and
// returns it; this is the only time it is shown. An older key of that side
// stops working at once. The key is 32 lowercase hex characters, the form
// those apps expect.
func (a *Auth) MakeArrKey(r *http.Request, side string) (string, error) {
	if side != "sonarr" && side != "radarr" {
		return "", ErrNoArrSide
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	k := hex.EncodeToString(b)
	who, _ := webserver.From(r.Context())
	if err := a.putArrKey(r.Context(), side, who.UserID, k); err != nil {
		return "", err
	}
	a.event(r, webserver.Actor(r.Context()), "api_key_created", "arr API key ("+side+") made", map[string]any{"old_app_side": side})
	return k, nil
}

// DeleteArrKey removes side's key; false when it had none.
func (a *Auth) DeleteArrKey(r *http.Request, side string) (bool, error) {
	n, err := a.removeArrKey(r.Context(), side)
	if err != nil || n == 0 {
		return false, err
	}
	a.event(r, webserver.Actor(r.Context()), "api_key_deleted", "arr API key ("+side+") removed", map[string]any{"old_app_side": side})
	return true, nil
}
