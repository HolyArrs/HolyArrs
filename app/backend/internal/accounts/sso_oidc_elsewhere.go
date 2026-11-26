package accounts

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// A sign-in that starts or comes back at another address, or in another
// browser, than the provider's answer is for.

// oidcRefusal is errOIDCFlow with the check that failed, in plain words for
// the log. The words never reach the browser and hold nothing from the request.
type oidcRefusal struct{ why string }

func (e *oidcRefusal) Error() string { return errOIDCFlow.Error() }
func (e *oidcRefusal) Unwrap() error { return errOIDCFlow }

var (
	errOIDCNoState  = &oidcRefusal{"no state, or more than one"}
	errOIDCNoCookie = &oidcRefusal{"no sign-in cookie"}
	errOIDCUnknown  = &oidcRefusal{"unknown or expired state"}
	errOIDCCookie   = &oidcRefusal{"cookie does not match"}
)

// oidcRefusalLog keeps a flood of refused callbacks (nobody has signed in
// yet) from filling the log: a few lines a minute are enough to see the cause.
var oidcRefusalLog struct {
	sync.Mutex
	from time.Time
	n    int
}

const oidcRefusalLogPerMinute = 20

// logOIDCRefusal says which check refused a callback. why is one of the fixed
// phrases: never the state, code, cookie, query or address.
func logOIDCRefusal(now time.Time, why string) {
	l := &oidcRefusalLog
	l.Lock()
	if now.Sub(l.from) >= time.Minute || now.Before(l.from) {
		l.from, l.n = now, 0
	}
	l.n++
	n := l.n
	l.Unlock()
	if n <= oidcRefusalLogPerMinute {
		log.Printf("auth: single sign-on: callback refused: %s", why)
	}
}

// oidcBrowserNavigation reports a page load in a browser (the provider sending
// the person back), as opposed to a script or API client reading the answer.
func oidcBrowserNavigation(r *http.Request) bool {
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" {
		return mode == "navigate"
	}
	// Browsers that send no Sec-Fetch headers still ask for a page.
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// oidcElsewhere answers a sign-in started at another address of this server:
// the provider answers at the redirect URL's address, where a cookie set here
// does not exist. No flow and no cookie; the page goes on at that address
// (built from the configured redirect URL only, never from the request), and
// a real new sign-in asked for stays one there. Linking needs the session
// too, which is also only here, so the page just names the address.
func oidcElsewhere(o *oidcRuntime, linking, fresh bool) map[string]string {
	at := o.redirect.Scheme + "://" + o.redirect.Host + strings.TrimSuffix(o.redirect.Path, "/api/v1/auth/oidc/callback")
	if linking {
		return map[string]string{"link_at": at}
	}
	at += "/login?sso=start"
	if fresh {
		at += "&fresh=1"
	}
	return map[string]string{"continue_at": at}
}

// oidcOnRedirectOrigin reports a request made at the redirect URL's own
// scheme, host and port: the only address whose cookies the callback sees.
func (a *Auth) oidcOnRedirectOrigin(r *http.Request, o *oidcRuntime) bool {
	scheme := "http"
	if r.TLS != nil || a.behindHTTPS(r) {
		scheme = "https"
	}
	host := &url.URL{Scheme: scheme, Host: r.Host}
	return strings.EqualFold(scheme, o.redirect.Scheme) && oidcOriginHost(host) == oidcOriginHost(o.redirect)
}
