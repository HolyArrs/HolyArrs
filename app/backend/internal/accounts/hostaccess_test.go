package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// These tests cover sign-in behind HTTPS proxies and tailscale serve, the
// split 403 messages, and home host names (first-run setup on tower.local,
// the remembered setup name and the "Allowed host names" setting).

// hostDo sends a request to the fixture with its own Host header.
func hostDo(t *testing.T, c *http.Client, method, rawURL, host, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, rawURL, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if host != "" {
		req.Host = host
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// First run has no setup code.
const setupBody = `{"name":"luis","password":"` + pw + `"}`

// setupOn runs first-run setup with the Host header set to host.
func setupOn(t *testing.T, f *fixture, c *http.Client, host string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	return hostDo(t, c, "POST", f.srv.URL+"/api/v1/auth/setup", host, setupBody, hdr)
}

func csrfOf(t *testing.T, body string) string {
	t.Helper()
	var out struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.CSRF == "" {
		t.Fatalf("no csrf token: %s", body)
	}
	return out.CSRF
}

func loginOn(t *testing.T, f *fixture, c *http.Client, host string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	return hostDo(t, c, "POST", f.srv.URL+"/api/v1/auth/login", host, `{"name":"luis","password":"`+pw+`"}`, hdr)
}

// gatedOn is the gated fixture on an existing store: a restart of the app.
func gatedOn(t *testing.T, s *database.Store, opt Options) *fixture {
	t.Helper()
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	c := newClock()
	opt.Now, opt.Vault, opt.fastHash = c.now, v, true
	a := New(s, opt)
	m := http.NewServeMux()
	m.HandleFunc("GET /", okHandler)
	m.HandleFunc("GET /api/v1/titles", okHandler)
	m.HandleFunc("POST /api/v1/titles", okHandler)
	m.HandleFunc("GET /api/v1/not-in-the-table", okHandler)
	a.Routes(m)
	srv := httptest.NewServer(a.Middleware(webserver.RecordRouteHits(Gate(m))))
	t.Cleanup(srv.Close)
	return &fixture{srv: srv, a: a, s: s, clock: c}
}

// shareCookies copies the cookies the jar keeps for host to the fixture's
// own address (the jar follows the Host header), so the same session can
// be used on another name.
func shareCookies(t *testing.T, f *fixture, c *http.Client, host string) {
	t.Helper()
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	c.Jar.SetCookies(mustURL(f.srv.URL+"/"), c.Jar.Cookies(mustURL("http://"+h+"/")))
}

func errorOf(body string) string {
	var out struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	return out.Error
}

// ---- Fix-first 6: HTTPS proxy, tunnel or tailscale serve ----

// A browser on https://holyarrs.example behind a proxy that talks plain
// HTTP to HolyArrs, with no HOLYARRS_TRUSTED_PROXIES: the browser's Origin
// says https, the server sees http. Same host, so it is the same site.
func TestSetupBehindHTTPSProxyWithoutTrustedProxies(t *testing.T) {
	f := gated(t, Options{AllowedHosts: []string{"holyarrs.example"}})
	c := client(t)
	hdr := map[string]string{"Origin": "https://holyarrs.example", "Sec-Fetch-Site": "same-origin", "X-Forwarded-Proto": "https"}
	resp, body := setupOn(t, f, c, "holyarrs.example", hdr)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup behind an HTTPS proxy: %d %s", resp.StatusCode, body)
	}
	csrf := csrfOf(t, body)
	hdr[CSRFHeader] = csrf
	if resp, out := hostDo(t, c, "POST", f.srv.URL+"/api/v1/titles", "holyarrs.example", "", hdr); resp.StatusCode != 200 {
		t.Fatalf("a write behind an HTTPS proxy: %d %s", resp.StatusCode, out)
	}
}

// tailscale serve: https://box.tail1234.ts.net in the browser, plain HTTP
// inside. Some browsers send Origin without Sec-Fetch-Site.
func TestSignInThroughTailscaleServe(t *testing.T) {
	const ts = "box.tail1234.ts.net"
	f := gated(t, Options{AllowedHosts: []string{ts}})
	ownerOf(t, f)
	c := client(t)
	resp, body := loginOn(t, f, c, ts, map[string]string{"Origin": "https://" + ts})
	if resp.StatusCode != 200 {
		t.Fatalf("sign-in through tailscale serve: %d %s", resp.StatusCode, body)
	}
	hdr := map[string]string{"Origin": "https://" + ts, "Sec-Fetch-Site": "same-origin", CSRFHeader: csrfOf(t, body)}
	if resp, out := hostDo(t, c, "POST", f.srv.URL+"/api/v1/titles", ts, "", hdr); resp.StatusCode != 200 {
		t.Fatalf("a write through tailscale serve: %d %s", resp.StatusCode, out)
	}
}

// The refusals stay: another site, another port, a null origin and a
// same-site (not same-origin) page are refused, and the 403 says it is
// another site, not a CSRF or host name problem.
func TestCrossSiteRefusalsKeepAndSayAnotherSite(t *testing.T) {
	f := gated(t, Options{AllowedHosts: []string{"holyarrs.example"}})
	ownerOf(t, f)
	for _, hdr := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"},
		{"Sec-Fetch-Site": "same-site", "Origin": "https://other.holyarrs.example"},
		{"Origin": "https://evil.example"},
		{"Origin": "https://holyarrs.example:444"},
		{"Origin": "null"},
	} {
		resp, body := loginOn(t, f, client(t), "holyarrs.example", hdr)
		msg := errorOf(body)
		if resp.StatusCode != 403 || !strings.Contains(msg, "another site") || strings.Contains(msg, "CSRF") || strings.Contains(msg, EnvAllowedHosts) {
			t.Errorf("sign-in with %v: %d %q, want 403 naming another site only", hdr, resp.StatusCode, msg)
		}
	}
}

// A signed-in write from another site is "another site"; the CSRF message
// is kept for a missing or wrong token only.
func TestSessionWriteSplits403(t *testing.T) {
	f := gated(t, Options{AllowedHosts: []string{"holyarrs.example"}})
	ownerOf(t, f)
	c := client(t)
	resp, body := loginOn(t, f, c, "holyarrs.example", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("login: %d %s", resp.StatusCode, body)
	}
	csrf := csrfOf(t, body)
	resp, out := hostDo(t, c, "POST", f.srv.URL+"/api/v1/titles", "holyarrs.example", "",
		map[string]string{CSRFHeader: csrf, "Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"})
	if msg := errorOf(out); resp.StatusCode != 403 || !strings.Contains(msg, "another site") || strings.Contains(msg, "CSRF") {
		t.Errorf("cross-site write: %d %q, want 403 another site", resp.StatusCode, msg)
	}
	resp, out = hostDo(t, c, "POST", f.srv.URL+"/api/v1/titles", "holyarrs.example", "", map[string]string{CSRFHeader: "wrong"})
	if msg := errorOf(out); resp.StatusCode != 403 || !strings.Contains(msg, "CSRF") {
		t.Errorf("wrong token: %d %q, want 403 CSRF", resp.StatusCode, msg)
	}
	other := client(t)
	other.Jar.SetCookies(mustURL("http://unknown.example/"), c.Jar.Cookies(mustURL("http://holyarrs.example/")))
	resp, out = hostDo(t, other, "POST", f.srv.URL+"/api/v1/titles", "unknown.example", "", map[string]string{CSRFHeader: csrf})
	if msg := errorOf(out); resp.StatusCode != 403 || strings.Contains(msg, "CSRF") || !strings.Contains(msg, "unknown.example") || !strings.Contains(msg, EnvAllowedHosts) {
		t.Errorf("unknown host write: %d %q, want 403 naming the host and %s", resp.StatusCode, msg, EnvAllowedHosts)
	}
}

// A refused host name is named (lower case, port dropped) with the setting
// to add it to, at sign-in and at setup.
func TestRefusedHostIsNamed(t *testing.T) {
	f := gated(t, Options{})
	c := client(t)
	resp, body := setupOn(t, f, c, "Media.Example.COM:8080", nil)
	msg := errorOf(body)
	if resp.StatusCode != 403 || !strings.Contains(msg, "media.example.com") || !strings.Contains(msg, EnvAllowedHosts) || strings.Contains(msg, "8080") {
		t.Fatalf("setup on an unknown host: %d %q", resp.StatusCode, msg)
	}
	ownerOf(t, f)
	resp, body = loginOn(t, f, client(t), "media.example.com", nil)
	msg = errorOf(body)
	if resp.StatusCode != 403 || !strings.Contains(msg, "media.example.com") || !strings.Contains(msg, EnvAllowedHosts) {
		t.Fatalf("sign-in on an unknown host: %d %q", resp.StatusCode, msg)
	}
}

// The echoed host is cleaned and capped, and the answer is JSON, not HTML.
func TestRefusedHostEchoIsCleanedAndCapped(t *testing.T) {
	f := gated(t, Options{})
	h := f.a.Middleware(http.NotFoundHandler())
	long := strings.Repeat("a", 300) + ".example"
	for _, host := range []string{long, "evil.example/<b>x</b>", "evil.example\"><script>"} {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{}`))
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		msg := errorOf(w.Body.String())
		if w.Code != 403 || !strings.Contains(msg, EnvAllowedHosts) {
			t.Errorf("%.40q: %d %q, want 403 naming %s", host, w.Code, msg, EnvAllowedHosts)
		}
		if strings.ContainsAny(msg, "<>\"") || strings.Contains(msg, "example/") || len(msg) > 300 || strings.Contains(msg, long) {
			t.Errorf("%.40q: echoed host not cleaned or capped: %q", host, msg)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%.40q: content type %q", host, ct)
		}
	}
}

// Allowed host entries may be pasted as addresses: the scheme, port and a
// trailing slash or dot are dropped.
func TestAllowedHostEntriesDropSchemeAndPort(t *testing.T) {
	f := gated(t, Options{AllowedHosts: []string{"https://Tower.Example:6262/", "http://nas.example."}})
	ownerOf(t, f)
	for _, host := range []string{"tower.example", "nas.example:6262"} {
		if resp, body := loginOn(t, f, client(t), host, nil); resp.StatusCode != 200 {
			t.Errorf("sign-in on %s: %d %s", host, resp.StatusCode, body)
		}
	}
}

// ---- Decisions 13 and 14: home names at first-run setup ----

// First-run setup accepts a home name (.local, .home.arpa, .internal,
// .localhost or no dot) with no proxy in between, and remembers it: sign-in
// and writes on that name work afterwards, also after a restart.
func TestSetupOnHomeNameIsRemembered(t *testing.T) {
	for _, host := range []string{"tower.local", "nas", "media.home.arpa", "box.internal", "dev.localhost", "Tower.Local.", "tower.local:6262"} {
		t.Run(host, func(t *testing.T) {
			f := gated(t, Options{})
			c := client(t)
			resp, body := setupOn(t, f, c, host, map[string]string{"Origin": "http://" + host})
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("setup on %s: %d %s", host, resp.StatusCode, body)
			}
			csrf := csrfOf(t, body)
			if resp, out := hostDo(t, c, "POST", f.srv.URL+"/api/v1/titles", host, "", map[string]string{CSRFHeader: csrf}); resp.StatusCode != 200 {
				t.Fatalf("write on the setup name %s: %d %s", host, resp.StatusCode, out)
			}
			if resp, out := loginOn(t, f, client(t), host, nil); resp.StatusCode != 200 {
				t.Fatalf("sign-in on the setup name %s: %d %s", host, resp.StatusCode, out)
			}
			again := gatedOn(t, f.s, Options{})
			if resp, out := loginOn(t, again, client(t), host, nil); resp.StatusCode != 200 {
				t.Fatalf("sign-in on the setup name %s after a restart: %d %s", host, resp.StatusCode, out)
			}
		})
	}
}

// Names that are not home names are never accepted at setup: .lan, tailnet
// names, onion names and public names. Nor is a home name when the request
// came through a proxy (a forwarding header, or any trusted proxy set).
func TestSetupRefusesOtherNames(t *testing.T) {
	cases := []struct {
		host string
		hdr  map[string]string
		opt  Options
	}{
		{host: "tower.lan"},
		{host: "box.tail1234.ts.net"},
		{host: "abc.onion"},
		{host: "evil.example"},
		{host: "x.local.evil.example"},
		{host: "tower.local", hdr: map[string]string{"X-Forwarded-For": "203.0.113.9"}},
		{host: "tower.local", hdr: map[string]string{"Forwarded": "for=203.0.113.9"}},
		{host: "tower.local", hdr: map[string]string{"X-Real-IP": "203.0.113.9"}},
		{host: "tower.local", opt: Options{TrustedProxies: []string{"10.9.9.9"}}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s %v %v", tc.host, tc.hdr, tc.opt.TrustedProxies), func(t *testing.T) {
			f := gated(t, tc.opt)
			resp, body := setupOn(t, f, client(t), tc.host, tc.hdr)
			if resp.StatusCode != 403 || !strings.Contains(errorOf(body), EnvAllowedHosts) {
				t.Fatalf("setup on %s: %d %s, want 403 naming %s", tc.host, resp.StatusCode, body, EnvAllowedHosts)
			}
			if _, _, ok := f.a.owner(context.Background()); ok {
				t.Fatal("an owner was made")
			}
		})
	}
}

// A home name refused at setup through a proxy is not remembered.
func TestSetupThroughProxyOnHomeNameRemembersNothing(t *testing.T) {
	f := gated(t, Options{})
	resp, body := setupOn(t, f, client(t), "tower.local", map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if resp.StatusCode != 403 {
		t.Fatalf("setup on tower.local through a proxy: %d %s, want 403", resp.StatusCode, body)
	}
	// Setup by IP; tower.local was never saved.
	ownerOf(t, f)
	resp, body = loginOn(t, f, client(t), "tower.local", nil)
	if msg := errorOf(body); resp.StatusCode != 403 || !strings.Contains(msg, "tower.local") || !strings.Contains(msg, EnvAllowedHosts) {
		t.Fatalf("sign-in on a name setup never confirmed: %d %q", resp.StatusCode, msg)
	}
}

// After setup, home names get no free pass: sign-in, writes and the
// local-network skip keep the plain list.
func TestHomeNamesNeedTheListAfterSetup(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8"]}`); resp.StatusCode != 200 {
		t.Fatalf("skip on: %d %s", resp.StatusCode, out)
	}
	if resp, out := hostDo(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "127.0.0.1", "", nil); resp.StatusCode != 200 {
		t.Fatalf("skip on an IP: %d %s", resp.StatusCode, out)
	}
	for _, host := range []string{"other.local", "nas", "box.internal"} {
		if resp, _ := hostDo(t, client(t), "GET", f.srv.URL+"/api/v1/titles", host, "", nil); resp.StatusCode != 401 {
			t.Errorf("local skip on %s: %d, want 401", host, resp.StatusCode)
		}
		resp, body := loginOn(t, f, client(t), host, nil)
		if msg := errorOf(body); resp.StatusCode != 403 || !strings.Contains(msg, host) {
			t.Errorf("sign-in on %s: %d %q, want 403 naming it", host, resp.StatusCode, msg)
		}
	}
}

// The setup route is the only one with the home-name exception: other
// public writes on a home name are refused before setup too.
func TestHomeNameExceptionIsSetupOnly(t *testing.T) {
	f := gated(t, Options{})
	resp, body := loginOn(t, f, client(t), "tower.local", nil)
	if msg := errorOf(body); resp.StatusCode != 403 || !strings.Contains(msg, "tower.local") {
		t.Fatalf("sign-in on tower.local before setup: %d %q", resp.StatusCode, msg)
	}
	resp, body = hostDo(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/", "tower.local", setupBody, nil)
	if resp.StatusCode == http.StatusCreated {
		t.Fatalf("a near-miss setup path made the owner: %s", body)
	}
}

// ---- Decision 14: the "Allowed host names" setting ----

func allowedHostsOf(t *testing.T, f *fixture, p person) []string {
	t.Helper()
	resp, out := p.do(t, f, "GET", "/api/v1/auth/settings", "")
	if resp.StatusCode != 200 {
		t.Fatalf("settings: %d %s", resp.StatusCode, out)
	}
	var v struct {
		AllowedHosts *[]string `json:"allowed_hosts"`
	}
	jsonOf(t, out, &v)
	if v.AllowedHosts == nil {
		t.Fatalf("settings have no allowed_hosts: %s", out)
	}
	return *v.AllowedHosts
}

func TestAllowedHostNamesSetting(t *testing.T) {
	f := gated(t, Options{})
	c := client(t)
	resp, body := setupOn(t, f, c, "tower.local", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup on tower.local: %d %s", resp.StatusCode, body)
	}
	// The setup name shows in Settings.
	shareCookies(t, f, c, "tower.local")
	own := person{c, csrfOf(t, body)}
	if got := allowedHostsOf(t, f, own); len(got) != 1 || got[0] != "tower.local" {
		t.Fatalf("allowed hosts after setup: %v", got)
	}
	// Entries pasted as addresses are cleaned; bad ones are refused.
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"allowed_hosts":["tower.local","https://Media.Example:8443/"]}`); resp.StatusCode != 200 {
		t.Fatalf("save names: %d %s", resp.StatusCode, out)
	}
	if got := allowedHostsOf(t, f, own); strings.Join(got, ",") != "tower.local,media.example" {
		t.Fatalf("saved names: %v", got)
	}
	if resp, out := loginOn(t, f, client(t), "media.example", nil); resp.StatusCode != 200 {
		t.Fatalf("sign-in on a saved name: %d %s", resp.StatusCode, out)
	}
	for _, bad := range []string{`"*.example"`, `"a b"`, `"a/b.example"`, `"evil.example?x"`, `"user@evil.example"`, `"` + strings.Repeat("a", 260) + `"`} {
		if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"allowed_hosts":["tower.local",`+bad+`]}`); resp.StatusCode != 400 {
			t.Errorf("bad name %s: %d %s, want 400", bad, resp.StatusCode, out)
		}
	}
}

// Removing the name you are using now would lock you out: 409. From an IP
// address it is fine. Names set by HOLYARRS_ALLOWED_HOSTS cannot be removed
// in Settings.
func TestAllowedHostNamesLockOutGuardAndEnvNames(t *testing.T) {
	f := gated(t, Options{AllowedHosts: []string{"env.example"}})
	c := client(t)
	resp, body := setupOn(t, f, c, "tower.local", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup on tower.local: %d %s", resp.StatusCode, body)
	}
	csrf := csrfOf(t, body)
	resp, out := hostDo(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", "tower.local", `{"allowed_hosts":[]}`, map[string]string{CSRFHeader: csrf})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("removing the name in use: %d %s, want 409", resp.StatusCode, out)
	}
	if resp, out := loginOn(t, f, client(t), "tower.local", nil); resp.StatusCode != 200 {
		t.Fatalf("tower.local after a refused removal: %d %s", resp.StatusCode, out)
	}
	shareCookies(t, f, c, "tower.local")
	own := person{c, csrf}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"allowed_hosts":[]}`); resp.StatusCode != 200 {
		t.Fatalf("removing names from an IP address: %d %s", resp.StatusCode, out)
	}
	if resp, _ := loginOn(t, f, client(t), "tower.local", nil); resp.StatusCode != 403 {
		t.Fatalf("tower.local after removal: %d, want 403", resp.StatusCode)
	}
	if resp, out := loginOn(t, f, client(t), "env.example", nil); resp.StatusCode != 200 {
		t.Fatalf("a name from %s after clearing Settings: %d %s", EnvAllowedHosts, resp.StatusCode, out)
	}
}

// The setting needs a browser session with a fresh password: API keys are
// refused, and an old session is asked to confirm its password.
func TestAllowedHostNamesNeedFreshPassword(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	_, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"script","scopes":["admin"]}`)
	var k struct {
		Key string `json:"key"`
	}
	jsonOf(t, out, &k)
	if resp, out := call(t, &http.Client{}, "PUT", f.srv.URL+"/api/v1/auth/settings", `{"allowed_hosts":["evil.example"]}`, map[string]string{APIKeyHeader: k.Key}); resp.StatusCode != 403 {
		t.Fatalf("API key saved allowed hosts: %d %s", resp.StatusCode, out)
	}
	own = signIn(t, f, "luis") // a password sign-in; the setup session stays recent for an hour
	f.clock.add(11 * time.Minute)
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"allowed_hosts":["evil.example"]}`); resp.StatusCode != 403 || !strings.Contains(out, "reauth") {
		t.Fatalf("old session saved allowed hosts: %d %s", resp.StatusCode, out)
	}
	if resp, _ := loginOn(t, f, client(t), "evil.example", nil); resp.StatusCode != 403 {
		t.Fatalf("a refused save still let evil.example in: %d", resp.StatusCode)
	}
}

// Settings show and save the names, without any home-name setup first.
func TestAllowedHostNamesSaveFromIP(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if got := allowedHostsOf(t, f, own); len(got) != 0 {
		t.Fatalf("allowed hosts after an IP setup: %v", got)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"allowed_hosts":["nas.example"]}`); resp.StatusCode != 200 {
		t.Fatalf("save names: %d %s", resp.StatusCode, out)
	}
	if got := allowedHostsOf(t, f, own); len(got) != 1 || got[0] != "nas.example" {
		t.Fatalf("saved names: %v", got)
	}
	if resp, out := loginOn(t, f, client(t), "nas.example", nil); resp.StatusCode != 200 {
		t.Fatalf("sign-in on a saved name: %d %s", resp.StatusCode, out)
	}
	// Saved at once, with no restart, and kept after one.
	again := gatedOn(t, f.s, Options{})
	if resp, out := loginOn(t, again, client(t), "nas.example", nil); resp.StatusCode != 200 {
		t.Fatalf("sign-in on a saved name after a restart: %d %s", resp.StatusCode, out)
	}
}
