package accounts

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func originGuardAuth(t *testing.T, trustedProxy bool) *Auth {
	t.Helper()
	a := &Auth{hosts: map[string]bool{"holyarrs.example": true}}
	if trustedProxy {
		_, n, err := net.ParseCIDR("127.0.0.1/32")
		if err != nil {
			t.Fatal(err)
		}
		a.proxies = []*net.IPNet{n}
	}
	return a
}

func originRequest(target, origin string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, target, nil)
	r.Host = "holyarrs.example"
	r.RemoteAddr = "127.0.0.1:32100"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

// passes runs a public write through Middleware: true when it reaches the
// handler.
func passes(a *Auth, r *http.Request) bool {
	reached := false
	a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(httptest.NewRecorder(), r)
	return reached
}

// The origin check compares the browser's Origin host and port with the
// Host header (Go's cross-origin protection). The scheme is not guessed:
// behind an HTTPS proxy, tunnel or tailscale serve the server sees plain
// HTTP while the browser says https, and that is the same site.
func TestGuardedComparesOriginHostNotGuessedScheme(t *testing.T) {
	a := originGuardAuth(t, false)
	const login = "/api/v1/auth/login"
	if !passes(a, originRequest("http://holyarrs.example"+login, "https://holyarrs.example")) {
		t.Fatal("HTTPS origin refused for a plain HTTP request (HTTPS proxy without trusted proxies)")
	}
	if !passes(a, originRequest("https://holyarrs.example"+login, "https://holyarrs.example")) {
		t.Fatal("HTTPS origin refused for a direct HTTPS request")
	}
	if !passes(a, originRequest("http://holyarrs.example"+login, "http://holyarrs.example")) {
		t.Fatal("HTTP origin refused for a plain HTTP request")
	}
	if passes(a, originRequest("http://holyarrs.example"+login, "https://holyarrs.example:444")) {
		t.Fatal("origin with a different port was accepted")
	}
	if passes(a, originRequest("http://holyarrs.example"+login, "https://evil.example")) {
		t.Fatal("origin from another host was accepted")
	}
	if passes(a, originRequest("http://holyarrs.example"+login, "null")) {
		t.Fatal("null origin was accepted")
	}
	if !passes(a, originRequest("http://holyarrs.example"+login, "")) {
		t.Fatal("missing Origin no longer keeps the script-client baseline")
	}
	r := originRequest("http://holyarrs.example"+login, "https://holyarrs.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if passes(a, r) {
		t.Fatal("Sec-Fetch-Site cross-site was accepted")
	}
	r = originRequest("http://holyarrs.example"+login, "")
	r.Header.Set("Sec-Fetch-Site", "same-site")
	if passes(a, r) {
		t.Fatal("Sec-Fetch-Site same-site was accepted")
	}
}

// A forwarded scheme changes nothing in the origin check, trusted or not.
func TestGuardedIgnoresForwardedProto(t *testing.T) {
	for _, trusted := range []bool{true, false} {
		a := originGuardAuth(t, trusted)
		r := originRequest("http://holyarrs.example/api/v1/auth/login", "https://holyarrs.example")
		r.Header.Set("X-Forwarded-Proto", "https")
		if !passes(a, r) {
			t.Fatalf("trusted=%v: HTTPS origin behind an HTTPS proxy was refused", trusted)
		}
		r = originRequest("http://holyarrs.example/api/v1/auth/login", "https://evil.example")
		r.Header.Set("X-Forwarded-Proto", "https")
		if passes(a, r) {
			t.Fatalf("trusted=%v: another site was accepted with X-Forwarded-Proto", trusted)
		}
	}
}
