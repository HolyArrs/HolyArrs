package accounts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

type fixture struct {
	srv   *httptest.Server
	a     *Auth
	s     *database.Store
	clock *fakeClock
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time                    { return c.t }
func (c *fakeClock) add(d time.Duration)               { c.t = c.t.Add(d) }
func newClock() *fakeClock                             { return &fakeClock{t: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)} }
func okHandler(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("inside")) }

func setup(t *testing.T, opt Options) *fixture {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c := newClock()
	if opt.Now == nil {
		opt.Now = c.now
	}
	opt.fastHash = true
	a := New(s, opt)
	t.Cleanup(a.Close)
	m := http.NewServeMux()
	m.HandleFunc("GET /{$}", okHandler)
	m.HandleFunc("GET /api/v1/strong", func(w http.ResponseWriter, r *http.Request) {
		if webserver.RequireStrong(w, r) {
			okHandler(w, r)
		}
	})
	m.HandleFunc("GET /api/v1/titles", okHandler)
	m.HandleFunc("POST /api/v1/titles", okHandler)
	m.HandleFunc("GET /healthz", okHandler)
	a.Routes(m)
	srv := httptest.NewServer(a.Middleware(webserver.RecordRouteHits(m)))
	t.Cleanup(srv.Close)
	return &fixture{srv: srv, a: a, s: s, clock: c}
}

func client(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func call(t *testing.T, c *http.Client, method, url, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
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

const pw = "a long test passphrase"

func setupOwner(t *testing.T, f *fixture, c *http.Client) string {
	t.Helper()
	resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("setup: %d %s", resp.StatusCode, body)
	}
	var out struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.CSRF == "" {
		t.Fatalf("no csrf token: %s", body)
	}
	return out.CSRF
}

func TestEverythingNeedsSignIn(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	resp, _ := call(t, c, "GET", f.srv.URL+"/", "", nil)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("page: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, _ = call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil)
	if resp.StatusCode != 401 {
		t.Fatalf("api: %d", resp.StatusCode)
	}
	for _, p := range []string{"/healthz", "/login"} {
		resp, _ = call(t, c, "GET", f.srv.URL+p, "", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("%s should be open: %d", p, resp.StatusCode)
		}
	}
}

func TestFirstRunCreatesOneOwnerOnly(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	_, body := call(t, c, "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if !strings.Contains(body, `"needs_setup":true`) {
		t.Fatalf("state: %s", body)
	}
	resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"luis","password":"abc"}`, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("short password accepted: %d", resp.StatusCode)
	}
	setupOwner(t, f, c)
	resp, _ = call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("signed in after setup: %d", resp.StatusCode)
	}
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"eve","password":"`+pw+`"}`, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second setup: %d", resp.StatusCode)
	}
	var hash string
	_ = f.s.Read().QueryRow("SELECT password_hash FROM users").Scan(&hash)
	if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, pw) {
		t.Fatalf("hash %q", hash)
	}
}

// route: POST /api/v1/auth/logout
func TestLoginCookieFlagsAndLogout(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	c := client(t)
	resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"wrong password"}`, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("wrong password: %d %s", resp.StatusCode, body)
	}
	resp, body = call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("login: %d %s", resp.StatusCode, body)
	}
	ck := resp.Header.Get("Set-Cookie")
	for _, want := range []string{CookieName + "=", "HttpOnly", "SameSite=Lax", "Path=/"} {
		if !strings.Contains(ck, want) {
			t.Fatalf("cookie %q lacks %q", ck, want)
		}
	}
	if strings.Contains(ck, "Secure") {
		t.Fatalf("plain http cookie should not be Secure: %q", ck)
	}
	var out struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	resp, _ = call(t, c, "POST", f.srv.URL+"/api/v1/auth/logout", "", map[string]string{CSRFHeader: out.CSRF})
	if resp.StatusCode != 204 {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Clear-Site-Data"); got != `"cache"` {
		t.Fatalf("logout Clear-Site-Data %q", got)
	}
	resp, _ = call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil)
	if resp.StatusCode != 401 {
		t.Fatalf("after logout: %d", resp.StatusCode)
	}
}

func TestForwardedProtoNeedsTrustedProxy(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`,
		map[string]string{"X-Forwarded-Proto": "https"})
	if strings.Contains(resp.Header.Get("Set-Cookie"), "Secure") {
		t.Fatal("X-Forwarded-Proto believed from an untrusted peer")
	}
}

func TestSecureCookieBehindHTTPS(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1"}})
	setupOwner(t, f, client(t))
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`,
		map[string]string{"X-Forwarded-Proto": "https"})
	if !strings.Contains(resp.Header.Get("Set-Cookie"), "Secure") {
		t.Fatalf("cookie not Secure behind https: %q", resp.Header.Get("Set-Cookie"))
	}
}

func TestSessionRotatesOnLogin(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	setupOwner(t, f, c)
	first := c.Jar.Cookies(mustURL(f.srv.URL))[0].Value
	resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	second := c.Jar.Cookies(mustURL(f.srv.URL))[0].Value
	if first == second {
		t.Fatal("session id did not change on login")
	}
	old := client(t)
	old.Jar.SetCookies(mustURL(f.srv.URL), []*http.Cookie{{Name: CookieName, Value: first}})
	if resp, _ := call(t, old, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("old session still works: %d", resp.StatusCode)
	}
}

func TestSessionRotatesWhileInUse(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	setupOwner(t, f, c)
	first := c.Jar.Cookies(mustURL(f.srv.URL))[0].Value
	f.clock.add(RotateEvery + time.Minute)
	if resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	if c.Jar.Cookies(mustURL(f.srv.URL))[0].Value == first {
		t.Fatal("session id not rotated")
	}
	f.clock.add(rotateGrace + time.Second)
	old := client(t)
	old.Jar.SetCookies(mustURL(f.srv.URL), []*http.Cookie{{Name: CookieName, Value: first}})
	if resp, _ := call(t, old, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("rotated-out session still works: %d", resp.StatusCode)
	}
}

func TestIdleAndAbsoluteExpiry(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	setupOwner(t, f, c)
	f.clock.add(IdleTimeout + time.Minute)
	if resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("idle session works: %d", resp.StatusCode)
	}
	c = client(t)
	resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	for elapsed := time.Duration(0); elapsed < AbsoluteTimeout; elapsed += IdleTimeout / 2 {
		f.clock.add(IdleTimeout / 2)
		call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil)
	}
	if resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("session past absolute limit works: %d", resp.StatusCode)
	}
}

func TestCSRFNeededForChanges(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/titles", "{}", nil); resp.StatusCode != 403 {
		t.Fatalf("no token: %d", resp.StatusCode)
	}
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/titles", "{}", map[string]string{CSRFHeader: "nope"}); resp.StatusCode != 403 {
		t.Fatalf("bad token: %d", resp.StatusCode)
	}
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/titles", "{}", map[string]string{CSRFHeader: csrf, "Origin": "http://evil.example"}); resp.StatusCode != 403 {
		t.Fatalf("cross origin: %d", resp.StatusCode)
	}
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/titles", "{}", map[string]string{CSRFHeader: csrf, "Origin": f.srv.URL}); resp.StatusCode != 200 {
		t.Fatalf("good token: %d", resp.StatusCode)
	}
	// Login and setup refuse a foreign Origin too.
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`,
		map[string]string{"Origin": "http://evil.example"}); resp.StatusCode != 403 {
		t.Fatalf("cross-origin login: %d", resp.StatusCode)
	}
}

func TestLoginBackOff(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	c := client(t)
	var last *http.Response
	for i := 0; i < freeTries+1; i++ {
		last, _ = call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"wrong password"}`, nil)
	}
	if last.StatusCode != http.StatusTooManyRequests || last.Header.Get("Retry-After") == "" {
		t.Fatalf("no back-off: %d", last.StatusCode)
	}
	// Even the right password waits during the back-off.
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil); resp.StatusCode != 429 {
		t.Fatalf("right password during back-off: %d", resp.StatusCode)
	}
	f.clock.add(MaxBackoff + time.Second)
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("after back-off: %d", resp.StatusCode)
	}
}

func TestAPIKeyShownOnceStoredHashed(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/keys", `{"label":"script"}`, map[string]string{CSRFHeader: csrf})
	if resp.StatusCode != 201 {
		t.Fatalf("make key: %d %s", resp.StatusCode, body)
	}
	var k struct {
		ID  int64  `json:"id"`
		Key string `json:"key"`
	}
	_ = json.Unmarshal([]byte(body), &k)
	if len(k.Key) < 32 {
		t.Fatalf("key %q", k.Key)
	}
	var stored string
	_ = f.s.Read().QueryRow("SELECT hash FROM api_keys").Scan(&stored)
	if stored == k.Key || strings.Contains(stored, k.Key) {
		t.Fatal("key stored in the clear")
	}
	_, list := call(t, c, "GET", f.srv.URL+"/api/v1/auth/keys", "", nil)
	if strings.Contains(list, k.Key) {
		t.Fatal("list shows the key again")
	}
	// A script with the key needs no cookie and no CSRF token.
	bare := &http.Client{}
	if resp, _ := call(t, bare, "POST", f.srv.URL+"/api/v1/titles", "{}", map[string]string{APIKeyHeader: k.Key}); resp.StatusCode != 200 {
		t.Fatalf("key request: %d", resp.StatusCode)
	}
	if resp, _ := call(t, bare, "GET", f.srv.URL+"/api/v1/titles", "", map[string]string{APIKeyHeader: k.Key + "x"}); resp.StatusCode != 401 {
		t.Fatalf("wrong key: %d", resp.StatusCode)
	}
	resp, _ = call(t, c, "DELETE", f.srv.URL+"/api/v1/auth/keys/"+itoa(k.ID), "", map[string]string{CSRFHeader: csrf})
	if resp.StatusCode != 204 {
		t.Fatalf("delete key: %d", resp.StatusCode)
	}
	if resp, _ := call(t, bare, "GET", f.srv.URL+"/api/v1/titles", "", map[string]string{APIKeyHeader: k.Key}); resp.StatusCode != 401 {
		t.Fatalf("deleted key works: %d", resp.StatusCode)
	}
}

func TestSkipLoginOnLocalNetworkIsOffByDefault(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	// The test server is on loopback, which counts as local.
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("local skip on by default: %d", resp.StatusCode)
	}
	_, body := call(t, c, "GET", f.srv.URL+"/api/v1/auth/settings", "", nil)
	if !strings.Contains(body, `"skip_login_local_network":false`) {
		t.Fatalf("settings: %s", body)
	}
	resp, _ := call(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`, map[string]string{CSRFHeader: csrf})
	if resp.StatusCode != 200 {
		t.Fatalf("turn on: %d", resp.StatusCode)
	}
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 200 {
		t.Fatalf("local skip on: %d", resp.StatusCode)
	}
	// Changes from a skipped request still need a same-origin request.
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/titles", "{}", map[string]string{"Origin": "http://evil.example"}); resp.StatusCode != 403 {
		t.Fatalf("cross-origin change with skip: %d", resp.StatusCode)
	}
}

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, pw) || CheckPassword(h, pw+"x") {
		t.Fatal("check wrong")
	}
	h2, _ := HashPassword(pw)
	if h == h2 {
		t.Fatal("no salt")
	}
	if CheckPassword("garbage", pw) {
		t.Fatal("garbage hash accepted")
	}
}

func TestCleanupDropsExpired(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	f.clock.add(AbsoluteTimeout + time.Hour)
	if err := f.a.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sessions").Scan(&n)
	if n != 0 {
		t.Fatalf("%d sessions left", n)
	}
}

func mustURL(s string) *url.URL { u, _ := url.Parse(s); return u }

// First run has no setup code: a leftover code file from
// an older version is removed at start, and a "code" field is ignored.
func TestSetupNeedsNoCode(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p := filepath.Join(s.Dir(), SetupCodeFile)
	if err := os.WriteFile(p, []byte("old-code\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := New(s, Options{fastHash: true})
	t.Cleanup(a.Close)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("old setup code file not removed at start: %v", err)
	}
	f := setup(t, Options{})
	body := `{"name":"luis","password":"` + pw + `","code":"anything"}`
	if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup", body, nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup with an ignored code: %d %s", resp.StatusCode, out)
	}
	if _, err := os.Stat(filepath.Join(f.s.Dir(), SetupCodeFile)); !os.IsNotExist(err) {
		t.Fatalf("a setup code file was written: %v", err)
	}
}

func hostCall(t *testing.T, c *http.Client, method, u, host, body string, hdr map[string]string) int {
	t.Helper()
	req, _ := http.NewRequest(method, u, strings.NewReader(body))
	req.Host = host
	if c.Jar != nil { // the jar follows Host, so send the session by hand
		for _, ck := range c.Jar.Cookies(mustURL(u)) {
			req.AddCookie(ck)
		}
		c = &http.Client{}
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestForeignHostRefused(t *testing.T) {
	f := setup(t, Options{AllowedHosts: []string{"holyarrs.home"}})
	c := client(t)
	csrf := setupOwner(t, f, c)
	if got := hostCall(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", "rebind.evil.example", `{"name":"luis","password":"`+pw+`"}`, nil); got != 403 {
		t.Fatalf("login with foreign host: %d", got)
	}
	if got := hostCall(t, c, "POST", f.srv.URL+"/api/v1/titles", "rebind.evil.example:80", "{}", map[string]string{CSRFHeader: csrf}); got != 403 {
		t.Fatalf("change with foreign host: %d", got)
	}
	if got := hostCall(t, c, "POST", f.srv.URL+"/api/v1/titles", "holyarrs.home", "{}", map[string]string{CSRFHeader: csrf}); got != 200 {
		t.Fatalf("change with allowed host: %d", got)
	}
	if got := hostCall(t, c, "POST", f.srv.URL+"/api/v1/titles", "127.0.0.1", "{}", map[string]string{CSRFHeader: csrf, "Sec-Fetch-Site": "cross-site"}); got != 403 {
		t.Fatalf("cross-site fetch: %d", got)
	}
	call(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`, map[string]string{CSRFHeader: csrf})
	if got := hostCall(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "rebind.evil.example", "", nil); got != 401 {
		t.Fatalf("local skip with foreign host: %d", got)
	}
}

func TestSetupRefusesForeignHost(t *testing.T) {
	f := setup(t, Options{})
	body := `{"name":"luis","password":"` + pw + `"}`
	if got := hostCall(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup", "rebind.evil.example", body, nil); got != 403 {
		t.Fatalf("setup with foreign host: %d", got)
	}
}

func TestLocalSkipIsNotStrong(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	call(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`, map[string]string{CSRFHeader: csrf})
	bare := client(t)
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v1/auth/keys", `{"label":"x"}`},
		{"GET", "/api/v1/auth/keys", ""},
		{"PUT", "/api/v1/auth/settings", `{"skip_login_local_network":false}`},
		{"GET", "/api/v1/strong", ""},
	} {
		if resp, _ := call(t, bare, tc.method, f.srv.URL+tc.path, tc.body, nil); resp.StatusCode != 403 {
			t.Fatalf("%s %s with local skip: %d", tc.method, tc.path, resp.StatusCode)
		}
	}
	if resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/strong", "", nil); resp.StatusCode != 200 {
		t.Fatalf("session is strong: %d", resp.StatusCode)
	}
}

func TestForwardedHeadersNeedTrustedProxy(t *testing.T) {
	for _, tc := range []struct {
		proxies []string
		hdr     map[string]string
		want    int
	}{
		{nil, map[string]string{"X-Forwarded-For": "10.0.0.5"}, 401},
		{nil, map[string]string{"X-Real-IP": "10.0.0.5"}, 401},
		{nil, map[string]string{"Forwarded": "for=10.0.0.5"}, 401},
		{[]string{"127.0.0.1"}, map[string]string{"X-Forwarded-For": "10.0.0.5"}, 200},
		{[]string{"127.0.0.0/8"}, map[string]string{"X-Forwarded-For": "10.0.0.5, 8.8.8.8"}, 401},
		{nil, nil, 200},
	} {
		f := setup(t, Options{TrustedProxies: tc.proxies})
		c := client(t)
		csrf := setupOwner(t, f, c)
		call(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`, map[string]string{CSRFHeader: csrf})
		if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", tc.hdr); resp.StatusCode != tc.want {
			t.Fatalf("%v %v: %d, want %d", tc.proxies, tc.hdr, resp.StatusCode, tc.want)
		}
	}
}

func TestParallelLoginsCannotSlipPastBackOff(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	var wg sync.WaitGroup
	var mu sync.Mutex
	checked := 0
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"wrong password"}`, nil)
			if resp.StatusCode == 401 {
				mu.Lock()
				checked++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if checked > freeTries {
		t.Fatalf("%d password checks ran; back-off allows %d", checked, freeTries)
	}
}
