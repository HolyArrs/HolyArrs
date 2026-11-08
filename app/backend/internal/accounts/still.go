package accounts

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
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
	family, err := sessionFamily(r.Context(), a.s.Read(), sum(ck.Value))
	if err != nil || family == "" {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), familyKey{}, family))
}

// Still reads again who signed in the request r, without touching the
// session. A long-lived stream (/live) asks it now and then, so a sign-out,
// an ended session, a disabled account or changed rights reach a stream
// that is already open. ok is false when that sign-in no longer works.
func (a *Auth) Still(r *http.Request) (webserver.Caller, bool) {
	who, ok := webserver.From(r.Context())
	if !ok {
		return webserver.Caller{}, false
	}
	switch who.Via {
	case "session":
		ck, err := r.Cookie(CookieName)
		if err != nil || ck.Value == "" {
			return webserver.Caller{}, false
		}
		var fresh webserver.Caller
		// The newest session of the family the stream opened with, else the
		// presented one. Signing out, ending sessions or disabling the
		// account removes the whole family, so nothing is found then.
		family, _ := r.Context().Value(familyKey{}).(string)
		perms, seen, expires, err := sessionStill(r.Context(), a.s.Read(), sum(ck.Value), family, &fresh)
		if err != nil {
			return webserver.Caller{}, false
		}
		now := a.now()
		if !now.Before(parseTime(expires)) || now.Sub(parseTime(seen)) > IdleTimeout {
			return webserver.Caller{}, false
		}
		a.fill(&fresh, perms)
		fresh.Via = "session"
		return fresh, true
	case "api_key":
		return a.checkKey(r.Context(), r.Header.Get(APIKeyHeader))
	case "local_network":
		id, _, ok := a.owner(r.Context())
		if !ok || id != who.UserID || !a.strictLocal(r) {
			return webserver.Caller{}, false
		}
		return who, true
	}
	return who, true
}
