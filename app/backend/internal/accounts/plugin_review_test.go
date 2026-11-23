package accounts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestPluginTokenRefusedThroughProxiesTable(t *testing.T) {
	a := setup(t, Options{}).a
	tok := IssuePluginToken("px", []string{"library.read"})
	t.Cleanup(func() { RevokePluginToken("px") })
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	for _, c := range []struct {
		name, header, value string
		want                int
	}{
		{"plain loopback", "", "", http.StatusTeapot},
		{"x-forwarded-for", "X-Forwarded-For", "8.8.8.8", http.StatusUnauthorized},
		{"x-real-ip", "X-Real-IP", "8.8.8.8", http.StatusUnauthorized},
		{"forwarded", "Forwarded", "for=8.8.8.8", http.StatusUnauthorized},
		{"plugin proxy mark", PluginProxyHeader, "x", http.StatusUnauthorized},
	} {
		r := httptest.NewRequest("GET", "/api/v1/titles", nil)
		r.RemoteAddr = "127.0.0.1:5000"
		r.Header.Set("Authorization", "Bearer "+tok)
		if c.header != "" {
			r.Header.Set(c.header, c.value)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: %d want %d", c.name, w.Code, c.want)
		}
	}
}

func TestRevokeCancelsCallsInFlight(t *testing.T) {
	a := setup(t, Options{}).a
	tok := IssuePluginToken("inflight", []string{"library.read"})
	started, done := make(chan struct{}), make(chan error, 1)
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			done <- r.Context().Err()
		case <-time.After(3 * time.Second):
			done <- nil
		}
	}))
	r := httptest.NewRequest("GET", "/api/v1/titles", nil).WithContext(context.Background())
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("Authorization", "Bearer "+tok)
	go h.ServeHTTP(httptest.NewRecorder(), r)
	<-started
	RevokePluginToken("inflight")
	if err := <-done; err == nil {
		t.Fatal("a call kept running after its plugin's token ended")
	}
}

func TestHandsOffRefusesFileChangesTable(t *testing.T) {
	old := PluginHandsOff
	t.Cleanup(func() { PluginHandsOff = old })
	on := true
	PluginHandsOff = func() bool { return on }
	m := http.NewServeMux()
	for r := range PluginScopes {
		m.HandleFunc(r, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	}
	g := Gate(m)
	IssuePluginToken("ho2", PluginScopeList())
	t.Cleanup(func() { RevokePluginToken("ho2") })
	for _, c := range []struct {
		method, path string
		refused      bool
	}{
		{"POST", "/api/v1/titles/1/rename", true},
		{"POST", "/api/v1/titles/1/move", true},
		{"POST", "/api/v1/titles/1/subtitles/clean", true},
		{"POST", "/api/v1/titles/1/versions/2/adopt", true},
		{"POST", "/api/v1/titles/1/files/2/keep-and-add", true},
		{"PUT", "/api/v1/titles/1", false},
		{"GET", "/api/v1/titles", false},
		{"POST", "/api/v1/titles/refresh", false},
	} {
		r := httptest.NewRequest(c.method, c.path, nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{Via: "plugin", PluginID: "ho2", Scopes: PluginScopeList()}))
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if (w.Code == http.StatusForbidden) != c.refused {
			t.Errorf("%s %s: %d", c.method, c.path, w.Code)
		}
	}
	for r, s := range PluginScopes {
		if s == "library.delete" || s == "downloads.delete" || s == "files.delete" {
			if !pluginTouchesFiles(r, s) {
				t.Errorf("%s (%s) not held off", r, s)
			}
		}
	}
}

func TestMarkNameBelongsToItsPlugin(t *testing.T) {
	m := IssuePluginMark("netbird")
	t.Cleanup(func() { RevokePluginMark("netbird") })
	r := markedReq("GET", "/", m)
	r.Host, r.Header[PluginNameHeader] = "box.netbird.cloud", []string{"box.netbird.cloud"}
	if !pluginHost(r) {
		t.Fatal("first name refused")
	}
	r2 := markedReq("GET", "/", m)
	r2.Host, r2.Header[PluginNameHeader] = "evil.example.com", []string{"evil.example.com"}
	if pluginHost(r2) {
		t.Error("a second outside name took over the plugin's mark")
	}
	if markOwner(r) != "netbird" {
		t.Error("owner")
	}
}

func TestLoopbackLosesLocalSkipWhileForwarderRuns(t *testing.T) {
	a := setup(t, Options{}).a
	a.skipLocal.Store(true)
	m := IssuePluginMark("fwd-only")
	_ = m
	t.Cleanup(func() { RevokePluginMark("fwd-only") })
	r := httptest.NewRequest("GET", "/api/v1/titles", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	if a.strictLocal(r) {
		t.Error("loopback got the local skip while a forwarding plugin runs")
	}
}
