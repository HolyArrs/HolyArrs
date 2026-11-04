package auth

import (
	"context"
	"net/http"
)

type familyKey struct{}

// WithSessionFamily notes, for a request that stays open (/live), which
// session family its cookie belongs to. The cookie it holds is replaced
// every hour while the browser goes on with a new one of the same family;
// Still follows that family, so a working sign-in keeps its stream.
func (a *Auth) WithSessionFamily(r *http.Request) *http.Request {
	ck, err := r.Cookie(CookieName)
	if err != nil || ck.Value == "" {
		return r
	}
	var family string
	if a.s.Read().QueryRowContext(r.Context(), "SELECT family FROM sessions WHERE hash = ?", sum(ck.Value)).Scan(&family) != nil || family == "" {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), familyKey{}, family))
}

// Still reads again who signed in the request r, without touching the
// session. A long-lived stream (/live) asks it now and then, so a sign-out,
// an ended session, a disabled account or changed rights reach a stream
// that is already open. ok is false when that sign-in no longer works.
func (a *Auth) Still(r *http.Request) (Who, bool) {
	who, ok := From(r.Context())
	if !ok {
		return Who{}, false
	}
	switch who.Via {
	case "session":
		ck, err := r.Cookie(CookieName)
		if err != nil || ck.Value == "" {
			return Who{}, false
		}
		var fresh Who
		var perms, seen, expires string
		// The newest session of the family the stream opened with, else the
		// presented one. Signing out, ending sessions or disabling the
		// account removes the whole family, so nothing is found then.
		q, arg := `WHERE s.hash = ?`, any(sum(ck.Value))
		if family, _ := r.Context().Value(familyKey{}).(string); family != "" {
			q, arg = `WHERE s.family = ? ORDER BY s.replaced, s.rotated_at DESC LIMIT 1`, family
		}
		err = a.s.Read().QueryRowContext(r.Context(), `SELECT `+userCols+`, s.last_seen_at, s.expires_at
			FROM sessions s JOIN users u ON u.id = s.user_id AND u.state = 'active' `+q, arg).
			Scan(&fresh.UserID, &fresh.Name, &fresh.Role, &perms, &fresh.TwoFactor, &fresh.MaxRating, &fresh.RatingCountry, &seen, &expires)
		if err != nil {
			return Who{}, false
		}
		now := a.now()
		if !now.Before(parseTime(expires)) || now.Sub(parseTime(seen)) > IdleTimeout {
			return Who{}, false
		}
		a.fill(&fresh, perms)
		fresh.Via = "session"
		return fresh, true
	case "api_key":
		return a.checkKey(r.Context(), r.Header.Get(APIKeyHeader))
	case "local_network":
		id, _, ok := a.owner(r.Context())
		if !ok || id != who.UserID || !a.skipLocal.Load() || !a.localPeer(r) {
			return Who{}, false
		}
		return who, true
	}
	return who, true
}
