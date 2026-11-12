package accounts

import (
	"net/http"
	"strings"
)

// LocalCookieName is the cookie a local visit gets. A local visit has no
// session, so this cookie is what its CSRF token hangs on: the token is a
// hash of the cookie value, handed out by /api/v1/auth/me, and every
// state-changing request must send it back in CSRFHeader. Another site can
// neither read the cookie nor the token.
const LocalCookieName = "holyarrs_local"

// localToken returns the CSRF token for this browser's local cookie, setting
// a fresh cookie when there is none (had false), except on a cross-site visit.
func (a *Auth) localToken(w http.ResponseWriter, r *http.Request) (csrf string, had bool) {
	if ck, err := r.Cookie(LocalCookieName); err == nil && localCookieOK(ck.Value) {
		return localCSRF(ck.Value), true
	}
	// A link from another site comes without the cookie even when the
	// browser has one; a new one would change the token under the tabs
	// already open. The page's own first call, same-site, makes it.
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return "", false
	}
	v := token(32)
	http.SetCookie(w, &http.Cookie{Name: LocalCookieName, Value: v, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: a.behindHTTPS(r), MaxAge: int(AbsoluteTimeout.Seconds())})
	return localCSRF(v), false
}

func localCSRF(v string) string { return sum("local-csrf:" + v) }

// localCookieOK accepts only values token(32) makes.
func localCookieOK(v string) bool {
	if len(v) != 43 {
		return false
	}
	return strings.Trim(v, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_") == ""
}
