package accounts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func markedReq(method, path, mark string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	r.RemoteAddr = "127.0.0.1:40000"
	r.Host = "holyarrs.tail1.ts.net"
	r.Header.Set(PluginProxyHeader, mark)
	r.Header.Set(PluginPeerHeader, "100.64.0.7")
	r.Header.Set(PluginNameHeader, "holyarrs.tail1.ts.net")
	return r
}

func TestPluginMarkCompare(t *testing.T) {
	if len(PluginMark()) < 16 || PluginMark() != PluginMark() {
		t.Fatal("mark must be a stable random token")
	}
	if !pluginMarked(markedReq("GET", "/", PluginMark())) || pluginMarked(markedReq("GET", "/", "wrong")) {
		t.Fatal("mark compare")
	}
}

func TestPluginSetupRefused(t *testing.T) {
	a := &Auth{hosts: map[string]bool{"localhost": true}}
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Errorf("%s reached handler", r.URL.Path) }))
	for _, c := range []struct{ m, p string }{{"POST", "/api/v1/auth/setup"}, {"POST", "/api/v1/auth/setup/plex/start"},
		{"POST", "/api/v1/auth/setup/plex/finish"}, {"POST", "/api/v1/auth/setup/media-server/jellyfin"}, {"GET", "/api/v1/auth/setup/discover"}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, markedReq(c.m, c.p, PluginMark()))
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Finish setup on the home network first.") {
			t.Errorf("%s %s = %d %s", c.m, c.p, w.Code, w.Body)
		}
	}
}

func TestPluginHostAndAddr(t *testing.T) {
	a := &Auth{hosts: map[string]bool{"localhost": true}}
	a.skipLocal.Store(true)
	good := markedReq("GET", "/", PluginMark())
	if !a.hostAllowed(good) {
		t.Error("own ts.net name with the mark refused")
	}
	other := markedReq("GET", "/", PluginMark())
	other.Host = "evil.example.com"
	if a.hostAllowed(other) || a.hostAllowed(markedReq("GET", "/", "wrong")) {
		t.Error("host allowed without matching name and mark")
	}
	addr, ok := a.clientAddr(good)
	if addr != "100.64.0.7" || ok || a.strictLocal(good) || a.localPeer(good) {
		t.Errorf("marked visit: addr %q ok %v", addr, ok)
	}
	if k, sure := a.clientKeySure(good); k != "ip:100.64.0.7" || !sure {
		t.Errorf("key %q sure %v", k, sure)
	}
	if a.eventAddr(good) != "100.64.0.7" {
		t.Error("event addr")
	}
	bad := markedReq("GET", "/", "wrong")
	if addr, ok := a.clientAddr(bad); addr == "100.64.0.7" || ok {
		t.Errorf("wrong mark: addr %q ok %v", addr, ok)
	}
	if _, sure := a.clientKeySure(bad); sure || a.eventAddr(bad) != "" {
		t.Error("wrong mark must not give a known place")
	}
}
