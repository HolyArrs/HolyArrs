package auth

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

func TestGuardedBindsOriginToEffectiveScheme(t *testing.T) {
	a := originGuardAuth(t, false)

	if a.guarded(originRequest("https://holyarrs.example/api", "http://holyarrs.example")) {
		t.Fatal("HTTP origin accepted for direct HTTPS request")
	}
	if !a.guarded(originRequest("https://holyarrs.example/api", "https://holyarrs.example:443")) {
		t.Fatal("HTTPS origin with the default port was refused")
	}
	if a.guarded(originRequest("https://holyarrs.example/api", "https://holyarrs.example:444")) {
		t.Fatal("HTTPS origin with a different port was accepted")
	}
	if a.guarded(originRequest("http://holyarrs.example/api", "https://holyarrs.example")) {
		t.Fatal("HTTPS origin accepted for direct HTTP request")
	}
	if !a.guarded(originRequest("http://holyarrs.example/api", "http://holyarrs.example:80")) {
		t.Fatal("HTTP origin with the default port was refused")
	}
	if !a.guarded(originRequest("http://holyarrs.example/api", "")) {
		t.Fatal("missing Origin no longer keeps the script-client baseline")
	}
}

func TestGuardedTrustsForwardedHTTPSOnlyFromConfiguredProxy(t *testing.T) {
	trusted := originGuardAuth(t, true)
	r := originRequest("http://holyarrs.example/api", "https://holyarrs.example")
	r.Header.Set("X-Forwarded-Proto", "https")
	if !trusted.guarded(r) {
		t.Fatal("HTTPS origin behind a trusted HTTPS proxy was refused")
	}

	untrusted := originGuardAuth(t, false)
	r = originRequest("http://holyarrs.example/api", "https://holyarrs.example")
	r.Header.Set("X-Forwarded-Proto", "https")
	if untrusted.guarded(r) {
		t.Fatal("untrusted X-Forwarded-Proto changed the effective request scheme")
	}
}
