package accounts

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Middleware lets only signed-in requests through.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The arr API takes only its own keys. Like an API key, such a
		// key is no browser credential, so the host and origin checks do
		// not apply (Seerr calls from a Docker neighbour by name). A
		// missing or wrong key gets a bare 401, as the old apps answer.
		if to, ok := arrLink(r); ok {
			http.Redirect(w, r, to, http.StatusFound)
			return
		}
		if side := ArrSide(r.URL.Path); side != "" {
			k, _ := arrKey(r)
			who, ok := a.checkArrKey(r.Context(), side, k)
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(webserver.ContextWith(r.Context(), who)))
			return
		}
		if aiPublic(r) {
			// AI app sign-in: no cookies are read or set, so no origin check.
			// The host name is still checked, since the answers echo it.
			if !a.hostAllowed(r) {
				a.logRefusedHost(r.Host)
				fail(w, http.StatusForbidden, hostRefusedMsg(r.Host))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if public(r) {
			// First-run setup checks the host name itself: a home name is
			// accepted there with no proxy in between (hosts.go).
			setup := r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/setup" || setupOutsidePath(r)
			if unsafeMethod(r.Method) && a.refuse(w, r, !setup) {
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		k := r.Header.Get(APIKeyHeader)
		if k == "" && mcpPath(r.URL.Path) {
			// AI clients send their key as a bearer token.
			if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				k = strings.TrimSpace(v)
			}
		}
		if k == "" && mcpPath(r.URL.Path) {
			// An AI app without a key is told where to sign in, even on the
			// local network or beside a browser session.
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+a.aiResourceMetadataURL(r)+`", scope="mcp"`)
			fail(w, http.StatusUnauthorized, "sign in: add HolyArrs in your AI app and allow it in the browser")
			return
		}
		if k != "" {
			who, ok := a.checkKey(r.Context(), k)
			if !ok {
				if mcpPath(r.URL.Path) {
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", resource_metadata="`+a.aiResourceMetadataURL(r)+`"`)
				}
				fail(w, http.StatusUnauthorized, "API key not accepted")
				return
			}
			next.ServeHTTP(w, r.WithContext(webserver.ContextWith(r.Context(), who)))
			return
		}
		if who, ok := a.session(w, r); ok {
			if unsafeMethod(r.Method) {
				if a.refuse(w, r, true) {
					return
				}
				if subtle.ConstantTimeCompare([]byte(r.Header.Get(CSRFHeader)), []byte(who.CSRF)) != 1 {
					fail(w, http.StatusForbidden, "missing or wrong CSRF token")
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(webserver.ContextWith(r.Context(), who)))
			return
		}
		if a.strictLocal(r) {
			// The picked person, with their own role and limits; nobody
			// (sign in) when they are gone or a guard holds.
			if who, _, ok := a.localCaller(r.Context()); ok {
				csrf, had := a.localToken(w, r)
				if unsafeMethod(r.Method) {
					if a.refuse(w, r, true) {
						return
					}
					if !had || subtle.ConstantTimeCompare([]byte(r.Header.Get(CSRFHeader)), []byte(csrf)) != 1 {
						fail(w, http.StatusForbidden, "missing or wrong CSRF token")
						return
					}
				}
				who.CSRF = csrf
				next.ServeHTTP(w, r.WithContext(webserver.ContextWith(r.Context(), who)))
				return
			}
		}
		if wantsPage(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		fail(w, http.StatusUnauthorized, "sign in first")
	})
}

// public routes need no sign-in.
func public(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "GET /healthz", "HEAD /healthz", "GET /readyz", "HEAD /readyz", "GET /login", "GET /api/v1/auth/state",
		"POST /api/v1/auth/login", "POST /api/v1/auth/setup", "POST /api/v1/auth/login/two-factor",
		"POST /api/v1/auth/oidc/start", "GET /api/v1/auth/oidc/callback", "POST /api/v1/auth/oidc/finish",
		"POST /api/v1/auth/forward/login", "GET /api/v1/public/signin-art",
		"POST /api/v1/auth/plex/start", "POST /api/v1/auth/plex/finish",
		"POST /api/v1/auth/media-server/jellyfin", "POST /api/v1/auth/media-server/emby",
		"POST /api/v1/auth/setup/plex/start", "POST /api/v1/auth/setup/plex/finish",
		"POST /api/v1/auth/setup/media-server/jellyfin", "POST /api/v1/auth/setup/media-server/emby",
		"GET /api/v1/auth/setup/discover":
		return true
	}
	// The sign-in page's pictures (the handler serves only its own art).
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/public/signin-pic/t/p/") {
		return true
	}
	// Invite links: the page and the two calls it makes.
	if rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/auth/invites/"); ok && rest != "" && !strings.Contains(rest, "/") &&
		(r.Method == http.MethodGet || r.Method == http.MethodPost) {
		return true
	}
	// Joining through an invite with Plex, Jellyfin or Emby.
	if r.Method == http.MethodPost && inviteOutsidePath(r.URL.Path) {
		return true
	}
	if rest, ok := strings.CutPrefix(r.URL.Path, "/invite/"); ok && rest != "" && !strings.Contains(rest, "/") && r.Method == http.MethodGet {
		return true
	}
	// The web app's own files hold no data; the sign-in page needs them.
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && webserver.IsAsset(r.URL.Path)
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

func (a *Auth) trustedProxy(ip net.IP) bool {
	for _, n := range a.proxies {
		if n.Contains(ip) {
			return true
		}
	}
	if saved := a.savedProxies.Load(); saved != nil {
		for _, n := range saved.nets {
			if n.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// localPeer says whether the request comes from a listed local network,
// reading the caller's address by the shared rule in clientAddr.
func (a *Auth) localPeer(r *http.Request) bool {
	addr, ok := a.clientAddr(r)
	return ok && a.isLocal(addr)
}

// strictLocal says whether the request may skip sign-in on the local
// network: the skip is on, the Host is an IP address or a home-only name,
// and the caller is on a listed local network. Forwarding headers count only
// from a trusted proxy; from any other peer they end the skip. A proxy's own
// address never skips: a proxy that drops the headers and sets Host to the
// upstream IP (nginx's default) would otherwise pass every internet visitor
// off as a home one. That covers trusted proxies and the untrusted one the
// proxy warning saw; a proxy that never sends any forwarding header and is
// not listed cannot be told apart from a home device, which is why the
// Settings card asks the owner to list it.
//
// The container's own gateway is the exception to that memory: on a Docker
// bridge every home device arrives as the gateway, so one device sending a
// forwarding header would end the skip for the whole household. There only
// the request that carries the header is refused.
func (a *Auth) strictLocal(r *http.Request) bool {
	if !a.skipLocal.Load() || r.Header.Get(outbound.SelfCallHeader) != "" || !a.hostAllowed(r) {
		return false
	}
	if h := normHost(r.Host); net.ParseIP(h) == nil && !homeName(h) {
		return false
	}
	ip := parseHostIP(r.RemoteAddr)
	if anyForwardHeader(r) {
		if ip == nil || !a.trustedProxy(ip) {
			a.warnUntrustedProxy(ip)
			return false
		}
		return a.localPeer(r)
	}
	if ip == nil || a.trustedProxy(ip) {
		return false
	}
	// The container's own gateway is how LAN visits arrive on a Docker
	// bridge; it counts while the skip is on (local_gateway.go).
	if ownGateway(ip) {
		return true
	}
	return !a.seenProxy(ip) && a.isLocal(ip.String())
}

// anyForwardHeader says whether the request carries any proxy forwarding header.
func anyForwardHeader(r *http.Request) bool {
	if r.Header.Get("Forwarded") != "" || r.Header.Get("X-Real-IP") != "" {
		return true
	}
	for k := range r.Header {
		if strings.HasPrefix(k, "X-Forwarded-") {
			return true
		}
	}
	return false
}

// behindHTTPS believes X-Forwarded-Proto only from a trusted proxy.
func (a *Auth) behindHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && a.trustedProxy(ip)
}

func wantsPage(r *http.Request) bool {
	return r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/live"
}

// mcpPath is the AI connection route, the only one a bearer key reaches.
func mcpPath(p string) bool { return p == "/api/v1/mcp" || p == "/mcp" }
