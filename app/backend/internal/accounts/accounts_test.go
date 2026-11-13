package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// gated is a fixture with the permission gate and a secrets vault, like the
// real program.
func gated(t *testing.T, opt Options) *fixture {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
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

func jsonOf(t *testing.T, body string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
}

type person struct {
	c    *http.Client
	csrf string
}

func (p person) do(t *testing.T, f *fixture, method, path, body string) (*http.Response, string) {
	t.Helper()
	return call(t, p.c, method, f.srv.URL+path, body, map[string]string{CSRFHeader: p.csrf})
}

func ownerOf(t *testing.T, f *fixture) person {
	c := client(t)
	return person{c, setupOwner(t, f, c)}
}

// invite makes an invite as by and returns the link's token.
func invite(t *testing.T, f *fixture, by person, body string) string {
	t.Helper()
	resp, out := by.do(t, f, "POST", "/api/v1/users/invites", body)
	if resp.StatusCode != 201 {
		t.Fatalf("invite: %d %s", resp.StatusCode, out)
	}
	var v struct {
		Path string `json:"path"`
	}
	jsonOf(t, out, &v)
	return strings.TrimPrefix(v.Path, "/invite/")
}

func join(t *testing.T, f *fixture, tok, name string) person {
	t.Helper()
	c := client(t)
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"`+name+`","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("join: %d %s", resp.StatusCode, out)
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &v)
	return person{c, v.CSRF}
}

// route: POST /api/v1/auth/login
// route: GET /api/v1/auth/me
// route: GET /api/v1/auth/invites/{token}
func TestInviteMakesAMemberOnce(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"role":"member","note":"for Ana"}`)
	resp, out := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/invites/"+tok, "", nil)
	if resp.StatusCode != 200 || !strings.Contains(out, `"role":"member"`) || !strings.Contains(out, "for Ana") {
		t.Fatalf("look: %d %s", resp.StatusCode, out)
	}
	resp, _ = call(t, client(t), "GET", f.srv.URL+"/invite/"+tok, "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("invite page needs no sign-in: %d", resp.StatusCode)
	}
	ana := join(t, f, tok, "ana")
	_, me := ana.do(t, f, "GET", "/api/v1/auth/me", "")
	if !strings.Contains(me, `"role":"member"`) || !strings.Contains(me, `"perms":["ask"]`) {
		t.Fatalf("me: %s", me)
	}
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"eve","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("used link worked again: %d", resp.StatusCode)
	}
	// The new member signs in with their own password.
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"ana","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("member sign-in: %d", resp.StatusCode)
	}
	var hash string
	_ = f.s.Read().QueryRow("SELECT hash FROM invites").Scan(&hash)
	if hash == tok || strings.Contains(hash, tok) {
		t.Fatal("invite token stored in the clear")
	}
}

func TestInviteEndsAndTakenNames(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"days":1}`)
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 409 {
		t.Fatalf("owner's name taken: %d", resp.StatusCode)
	}
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"LUIS","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 409 {
		t.Fatalf("names differ only in case: %d", resp.StatusCode)
	}
	f.clock.add(25 * time.Hour)
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"ana","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("ended invite worked: %d", resp.StatusCode)
	}
	resp, _ = own.do(t, f, "POST", "/api/v1/users/invites", `{"days":90}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stale local proof reached invite settings: %d", resp.StatusCode)
	}
	resp, out := own.do(t, f, "POST", "/api/v1/auth/reauth", `{"password":"`+pw+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reauth: %d %s", resp.StatusCode, out)
	}
	var fresh struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &fresh)
	own.csrf = fresh.CSRF
	resp, _ = own.do(t, f, "POST", "/api/v1/users/invites", `{"days":90}`)
	if resp.StatusCode != 400 {
		t.Fatalf("90-day invite: %d", resp.StatusCode)
	}
}

func TestGuessingInvitesBacksOff(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	c := client(t)
	var last int
	for i := range 8 {
		resp, _ := call(t, c, "GET", f.srv.URL+fmt.Sprintf("/api/v1/auth/invites/guess%d", i), "", nil)
		last = resp.StatusCode
		f.clock.add(10 * time.Millisecond)
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("guessing not slowed: %d", last)
	}
}

// route: GET /api/v1/users
// route: GET /api/v1/auth/settings
// route: GET /api/v1/auth/keys
func TestMemberPermissionsOnRoutes(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{}`), "ana")
	cases := []struct {
		method, path  string
		member, owner int
	}{
		{"GET", "/api/v1/titles", 200, 200},
		{"POST", "/api/v1/titles", 403, 200},
		{"GET", "/api/v1/auth/settings", 403, 200},
		{"PUT", "/api/v1/auth/settings", 403, 400},
		{"GET", "/api/v1/users", 403, 200},
		{"POST", "/api/v1/users/invites", 403, 201},
		{"GET", "/api/v1/auth/keys", 403, 200},
		{"GET", "/api/v1/not-in-the-table", 403, 200},
		{"GET", "/api/v1/auth/me", 200, 200},
		{"GET", "/settings", 200, 200}, // the page itself; its data calls are refused
	}
	for _, c := range cases {
		if resp, out := ana.do(t, f, c.method, c.path, "{}"); resp.StatusCode != c.member {
			t.Errorf("member %s %s: %d %s", c.method, c.path, resp.StatusCode, out)
		}
		if resp, out := own.do(t, f, c.method, c.path, "{}"); resp.StatusCode != c.owner {
			t.Errorf("owner %s %s: %d %s", c.method, c.path, resp.StatusCode, out)
		}
	}
}

func userID(t *testing.T, f *fixture, name string) int64 {
	var id int64
	if err := f.s.Read().QueryRow("SELECT id FROM users WHERE name = ?", name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// route: PUT /api/v1/users/{id}
func TestAdminsAndRoles(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	kid := join(t, f, invite(t, f, ad, `{"role":"member","perms":["ask","manage_library"]}`), "kid")
	if resp, _ := kid.do(t, f, "POST", "/api/v1/titles", ""); resp.StatusCode != 200 {
		t.Fatalf("granted manage_library not honoured: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "POST", "/api/v1/users/invites", `{"role":"admin"}`); resp.StatusCode != 403 {
		t.Fatalf("admin made an admin invite: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "POST", "/api/v1/users/invites", `{"perms":["settings"]}`); resp.StatusCode != 403 {
		t.Fatalf("admin gave settings they lack: %d", resp.StatusCode)
	}
	ownerID, adminID, kidID := userID(t, f, "luis"), userID(t, f, "ana"), userID(t, f, "kid")
	if resp, _ := ad.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", ownerID), `{"state":"disabled"}`); resp.StatusCode != 403 {
		t.Fatalf("admin disabled the owner: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", kidID), `{"role":"admin"}`); resp.StatusCode != 403 {
		t.Fatalf("admin promoted a member: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", ownerID), `{"role":"member"}`); resp.StatusCode != 403 {
		t.Fatalf("owner demoted themself: %d", resp.StatusCode)
	}
	if resp, out := ad.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", kidID), `{"perms":["ask"]}`); resp.StatusCode != 200 {
		t.Fatalf("admin edit member: %d %s", resp.StatusCode, out)
	}
	if resp, _ := kid.do(t, f, "POST", "/api/v1/titles", ""); resp.StatusCode != 403 {
		t.Fatalf("taken-back permission still works: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", kidID), `{"state":"disabled"}`); resp.StatusCode != 200 {
		t.Fatalf("disable: %d", resp.StatusCode)
	}
	if resp, _ := kid.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 401 {
		t.Fatalf("disabled member still signed in: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", adminID), `{"role":"member"}`); resp.StatusCode != 200 {
		t.Fatalf("owner demote admin: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "GET", "/api/v1/users", ""); resp.StatusCode != 403 {
		t.Fatalf("demoted admin still sees users: %d", resp.StatusCode)
	}
	_, out := own.do(t, f, "GET", "/api/v1/users", "")
	if !strings.Contains(out, `"name":"kid"`) || !strings.Contains(out, `"state":"disabled"`) || strings.Contains(out, "argon") {
		t.Fatalf("users: %s", out)
	}
}

// codeNow is the current app code for a base32 secret.
func codeNow(t *testing.T, f *fixture, secret string, offset int64) string {
	k, err := b32.DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return totpCode(k, f.clock.t.Unix()/totpStep+offset)
}

func TestTOTPMatchesRFC6238(t *testing.T) {
	// RFC 6238 appendix B, SHA-1, last six digits.
	key := []byte("12345678901234567890")
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		if got := totpCode(key, unix/30); got != want {
			t.Errorf("%d: %s, want %s", unix, got, want)
		}
	}
}

func turnOnTwoFactor(t *testing.T, f *fixture, p person) (string, []string) {
	t.Helper()
	resp, out := p.do(t, f, "POST", "/api/v1/auth/two-factor/setup", "")
	if resp.StatusCode != 200 {
		t.Fatalf("setup: %d %s", resp.StatusCode, out)
	}
	var s struct {
		Secret string `json:"secret"`
		URI    string `json:"uri"`
	}
	jsonOf(t, out, &s)
	if !strings.HasPrefix(s.URI, "otpauth://totp/") || !strings.Contains(s.URI, s.Secret) {
		t.Fatalf("uri %q", s.URI)
	}
	if resp, _ := p.do(t, f, "POST", "/api/v1/auth/two-factor/enable", `{"code":"000000"}`); resp.StatusCode != 400 {
		t.Fatalf("wrong code turned it on: %d", resp.StatusCode)
	}
	resp, out = p.do(t, f, "POST", "/api/v1/auth/two-factor/enable", `{"code":"`+codeNow(t, f, s.Secret, 0)+`"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("enable: %d %s", resp.StatusCode, out)
	}
	var b struct {
		Codes []string `json:"backup_codes"`
	}
	jsonOf(t, out, &b)
	if len(b.Codes) != backupCodeCount {
		t.Fatalf("codes %v", b.Codes)
	}
	return s.Secret, b.Codes
}

func ticketFor(t *testing.T, f *fixture, c *http.Client, name string) string {
	t.Helper()
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"`+name+`","password":"`+pw+`"}`, nil)
	var v struct {
		TwoFactor bool   `json:"two_factor"`
		Ticket    string `json:"ticket"`
	}
	jsonOf(t, out, &v)
	if resp.StatusCode != 200 || !v.TwoFactor || v.Ticket == "" {
		t.Fatalf("login: %d %s", resp.StatusCode, out)
	}
	if resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("signed in before the code: %d", resp.StatusCode)
	}
	return v.Ticket
}

// route: POST /api/v1/auth/login/two-factor
// route: POST /api/v1/auth/two-factor/setup
// route: POST /api/v1/auth/two-factor/enable
func TestTwoFactorSignIn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	secret, codes := turnOnTwoFactor(t, f, own)
	var stored string
	_ = f.s.Read().QueryRow("SELECT value FROM secrets WHERE kind = 'two_factor'").Scan(&stored)
	if stored == "" || strings.Contains(stored, secret) {
		t.Fatal("two-factor secret not encrypted")
	}

	c := client(t)
	tk := ticketFor(t, f, c, "luis")
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"123456"}`, nil); resp.StatusCode != 401 {
		t.Fatalf("wrong code: %d", resp.StatusCode)
	}
	f.clock.add(totpStep * time.Second)
	good := codeNow(t, f, secret, 0)
	if resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"`+good+`"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("right code: %d %s", resp.StatusCode, out)
	}
	if resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 200 {
		t.Fatalf("not signed in after code: %d", resp.StatusCode)
	}
	// The same code cannot be used twice.
	c2 := client(t)
	tk2 := ticketFor(t, f, c2, "luis")
	if resp, _ := call(t, c2, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk2+`","code":"`+good+`"}`, nil); resp.StatusCode != 401 {
		t.Fatalf("code replayed: %d", resp.StatusCode)
	}
	// A backup code works once.
	if resp, _ := call(t, c2, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk2+`","code":"`+codes[0]+`"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("backup code: %d", resp.StatusCode)
	}
	c3 := client(t)
	tk3 := ticketFor(t, f, c3, "luis")
	if resp, _ := call(t, c3, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk3+`","code":"`+codes[0]+`"}`, nil); resp.StatusCode != 401 {
		t.Fatalf("backup code reused: %d", resp.StatusCode)
	}
}

func TestTwoFactorTicketLocksOut(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	secret, _ := turnOnTwoFactor(t, f, own)
	c := client(t)
	tk := ticketFor(t, f, c, "luis")
	for range ticketTries {
		call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"000000"}`, nil)
	}
	f.clock.add(totpStep * time.Second)
	resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"`+codeNow(t, f, secret, 0)+`"}`, nil)
	if resp.StatusCode == 200 {
		t.Fatal("ticket still worked after too many wrong codes")
	}
	f.clock.add(ticketLife + MaxBackoff)
	tk = ticketFor(t, f, c, "luis")
	f.clock.add(ticketLife + time.Second)
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tk+`","code":"`+codeNow(t, f, secret, 0)+`"}`, nil); resp.StatusCode != 401 {
		t.Fatalf("old ticket worked: %d", resp.StatusCode)
	}
}

// route: POST /api/v1/users/{id}/two-factor/reset
// route: POST /api/v1/auth/two-factor/disable
func TestTwoFactorOffAndReset(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{}`), "ana")
	secret, _ := turnOnTwoFactor(t, f, ana)
	if resp, _ := ana.do(t, f, "POST", "/api/v1/auth/two-factor/disable", `{"code":"000000"}`); resp.StatusCode != 401 {
		t.Fatalf("turned off with a wrong code: %d", resp.StatusCode)
	}
	f.clock.add(totpStep * time.Second)
	if resp, _ := ana.do(t, f, "POST", "/api/v1/auth/two-factor/disable", `{"code":"`+codeNow(t, f, secret, 0)+`"}`); resp.StatusCode != 204 {
		t.Fatalf("turn off: %d", resp.StatusCode)
	}
	turnOnTwoFactor(t, f, ana)
	if resp, _ := own.do(t, f, "POST", fmt.Sprintf("/api/v1/users/%d/two-factor/reset", userID(t, f, "ana")), ""); resp.StatusCode != 204 {
		t.Fatalf("owner reset: %d", resp.StatusCode)
	}
	resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"ana","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 200 || strings.Contains(out, "ticket") {
		t.Fatalf("after reset: %d %s", resp.StatusCode, out)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM secrets WHERE kind = 'two_factor'").Scan(&n)
	if n != 0 {
		t.Fatalf("%d two-factor secrets left behind", n)
	}
}

func TestOwnerCanRequireTwoFactorForAdmins(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"require_two_factor_admins":true}`); resp.StatusCode != 200 {
		t.Fatalf("settings: %d %s", resp.StatusCode, out)
	}
	if resp, _ := ad.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 403 {
		t.Fatalf("admin without two-factor reached the Library: %d", resp.StatusCode)
	}
	if _, me := ad.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(me, `"must_two_factor":true`) {
		t.Fatalf("me: %s", me)
	}
	turnOnTwoFactor(t, f, ad)
	if resp, _ := ad.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 200 {
		t.Fatalf("admin with two-factor: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 200 {
		t.Fatalf("owner is not an admin: %d", resp.StatusCode)
	}
}

// plexTok is the fake Plex's sign-in token.
const plexTok = "ptok"

// fakePlex answers the PIN, user and resources calls like plex.tv does.
type fakePlex struct {
	srv       *httptest.Server
	confirmed atomic.Bool
	server    string
	calls     atomic.Int32
	// restricted makes the person a managed Plex Home user with limits.
	restricted atomic.Bool
}

func newFakePlex(t *testing.T) *fakePlex {
	p := &fakePlex{server: "machine-1"}
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, r *http.Request) {
		p.calls.Add(1)
		if r.Header.Get("X-Plex-Client-Identifier") == "" || r.Header.Get("X-Plex-Product") != "HolyArrs" {
			http.Error(w, "headers", 400)
			return
		}
		// The first pin is 77, later ones count up, like separate sign-ins.
		_, _ = fmt.Fprintf(w, `{"id":%d,"code":"ABCD"}`, 76+p.calls.Load())
	})
	m.HandleFunc("GET /api/v2/pins/77", func(w http.ResponseWriter, _ *http.Request) {
		var tok any // null until the person confirms on Plex
		if p.confirmed.Load() {
			tok = plexTok
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 77, "authToken": tok})
	})
	m.HandleFunc("GET /api/v2/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != plexTok {
			http.Error(w, "no", 401)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":4242,"uuid":"u","username":"kidplex","email":"kid@example.test","restricted":%t}`, p.restricted.Load())
	})
	m.HandleFunc("GET /api/v2/resources", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `[{"clientIdentifier":%q,"provides":"server"}]`, p.server)
	})
	p.srv = httptest.NewServer(webserver.RecordRouteHits(m))
	t.Cleanup(p.srv.Close)
	return p
}

func plexSignIn(t *testing.T, f *fixture, p *fakePlex) (*http.Client, int, string) {
	t.Helper()
	c := client(t)
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(out, "ABCD") || !strings.Contains(out, "/auth#?") {
		t.Fatalf("start: %d %s", resp.StatusCode, out)
	}
	p.calls.Store(0)
	p.confirmed.Store(false)
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil); resp.StatusCode != 202 {
		t.Fatalf("waiting: %d", resp.StatusCode)
	}
	// One check a second per pin: a second check at once is refused.
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil); resp.StatusCode != 429 {
		t.Fatalf("a check within a second: %d", resp.StatusCode)
	}
	f.clock.t = f.clock.t.Add(2 * time.Second)
	p.confirmed.Store(true)
	resp, out = call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil)
	return c, resp.StatusCode, out
}

// route: POST /api/v1/auth/plex/start
// route: POST /api/v1/auth/plex/finish
func TestPlexSignInJoinsAsMember(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil); resp.StatusCode != 404 {
		t.Fatalf("plex while off: %d", resp.StatusCode)
	}
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"list","allow":"someone-else"}}`)
	if _, code, out := plexSignIn(t, f, p); code != 403 {
		t.Fatalf("not on the list: %d %s", code, out)
	}
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"other-server","join":"everyone"},"confirm_everyone":true}`)
	if _, code, _ := plexSignIn(t, f, p); code != 403 {
		t.Fatalf("does not share the server: %d", code)
	}
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"list","allow":"KID@example.test"}}`)
	c, code, out := plexSignIn(t, f, p)
	if code != 200 {
		t.Fatalf("plex sign-in: %d %s", code, out)
	}
	_, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
	if !strings.Contains(me, `"name":"kidplex"`) || !strings.Contains(me, `"role":"member"`) {
		t.Fatalf("me: %s", me)
	}
	// The pin is used up; a second finish fails.
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":77}`, nil); resp.StatusCode != 404 {
		t.Fatalf("pin reused: %d", resp.StatusCode)
	}
	// Linked people come back even when joining is closed.
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"join":"nobody"}}`)
	if _, code, _ := plexSignIn(t, f, p); code != 200 {
		t.Fatalf("linked member back: %d", code)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users").Scan(&n)
	if n != 2 {
		t.Fatalf("%d users, want owner and one member", n)
	}
	var stored int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM settings WHERE value LIKE '%'||?||'%'", plexTok).Scan(&stored)
	if stored != 0 {
		t.Fatal("Plex token stored")
	}
}

// A managed Plex Home user joins with the same Kid limits an import gives.
func TestRestrictedPlexJoinsAsKid(t *testing.T) {
	p := newFakePlex(t)
	p.restricted.Store(true)
	f := gated(t, Options{PlexURL: p.srv.URL})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"everyone"},"confirm_everyone":true}`)
	c, code, out := plexSignIn(t, f, p)
	if code != 200 {
		t.Fatalf("plex sign-in: %d %s", code, out)
	}
	if _, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); !strings.Contains(me, `"max_rating":"PG"`) || !strings.Contains(me, `"perms":["ask"]`) {
		t.Fatalf("me: %s", me)
	}
	// The owner may still lift the limit by hand later; that stays theirs.
	if resp, out := own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", userID(t, f, "kidplex")), `{"max_rating":""}`); resp.StatusCode != 200 {
		t.Fatalf("lift the limit: %d %s", resp.StatusCode, out)
	}
}

func TestPlexPinBelongsToItsStarter(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"everyone"},"confirm_everyone":true}`)
	for range pinsPerIP {
		call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil); resp.StatusCode != 429 {
		t.Fatalf("too many pins: %d", resp.StatusCode)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/plex/finish", `{"id":12345}`, nil); resp.StatusCode != 404 {
		t.Fatalf("unknown pin: %d", resp.StatusCode)
	}
}

// fakeMediaServer answers like Jellyfin or Emby.
func fakeMediaServer(t *testing.T, logouts *atomic.Int32) *httptest.Server {
	m := http.NewServeMux()
	m.HandleFunc("POST /Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "MediaBrowser ") {
			http.Error(w, "auth header", 400)
			return
		}
		var b struct{ Username, Pw string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		switch {
		case b.Username == "mum" && b.Pw == "right":
			_, _ = w.Write([]byte(`{"User":{"Id":"abc123","Name":"Mum","Policy":{"IsDisabled":false}},"AccessToken":"tok"}`))
		case b.Username == "off" && b.Pw == "right":
			_, _ = w.Write([]byte(`{"User":{"Id":"off1","Name":"off","Policy":{"IsDisabled":true}},"AccessToken":"tok"}`))
		case b.Username == "kid" && b.Pw == "right":
			// Parental controls on the server: this person may only watch some ratings.
			_, _ = w.Write([]byte(`{"User":{"Id":"kid1","Name":"Kid","Policy":{"IsDisabled":false,"MaxParentalRating":10}},"AccessToken":"tok"}`))
		case b.Username == "down":
			// A proxy in front says the server behind it is down.
			http.Error(w, "Service Unavailable", 503)
		case b.Username == "boom":
			// An odd server that turns down wrong passwords with 500.
			http.Error(w, "database is locked", 500)
		default:
			http.Error(w, "Error processing request.", 401)
		}
	})
	m.HandleFunc("POST /Sessions/Logout", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), `Token="tok"`) {
			logouts.Add(1)
		}
		w.WriteHeader(204)
	})
	s := httptest.NewServer(webserver.RecordRouteHits(m))
	t.Cleanup(s.Close)
	return s
}

// route: POST /api/v1/auth/media-server/{kind}
// route: GET /api/v1/auth/state
func TestMediaServerSignIn(t *testing.T) {
	for _, kind := range []string{"jellyfin", "emby"} {
		t.Run(kind, func(t *testing.T) {
			var logouts atomic.Int32
			ms := fakeMediaServer(t, &logouts)
			f := gated(t, Options{})
			own := ownerOf(t, f)
			login := func(name, pass string) (*http.Client, int) {
				c := client(t)
				b, _ := json.Marshal(map[string]string{"name": name, "password": pass})
				resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/media-server/"+kind, string(b), nil)
				return c, resp.StatusCode
			}
			if _, code := login("mum", "right"); code != 404 {
				t.Fatalf("while off: %d", code)
			}
			if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"`+kind+`":{"on":true,"url":"file:///etc"}}`); resp.StatusCode != 400 {
				t.Fatalf("odd address accepted: %d %s", resp.StatusCode, out)
			}
			own.do(t, f, "PUT", "/api/v1/auth/settings", `{"`+kind+`":{"on":true,"url":"`+ms.URL+`/","join":"everyone"},"confirm_everyone":true}`)
			if _, code := login("mum", "wrong"); code != 401 {
				t.Fatalf("wrong password: %d", code)
			}
			if _, code := login("off", "right"); code != 403 {
				t.Fatalf("disabled on the server: %d", code)
			}
			c, code := login("mum", "right")
			if code != 200 {
				t.Fatalf("sign-in: %d", code)
			}
			_, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
			if !strings.Contains(me, `"name":"Mum"`) || !strings.Contains(me, `"role":"member"`) || !strings.Contains(me, `"max_rating":""`) {
				t.Fatalf("me: %s", me)
			}
			// Someone the server limits joins as a Kid, but not while sign-in
			// is skipped at home (they would be the owner there).
			// A network the test client is not on, so its sign-ins stay ordinary.
			if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["10.0.0.0/8"]}`); resp.StatusCode != 200 {
				t.Fatalf("skip on: %d %s", resp.StatusCode, out)
			}
			if _, code := login("kid", "right"); code != 403 {
				t.Fatalf("kid joined with skip on: %d", code)
			}
			own.do(t, f, "PUT", "/api/v1/auth/settings", `{"skip_login_local_network":false}`)
			kc, code := login("kid", "right")
			if code != 200 {
				t.Fatalf("kid sign-in: %d", code)
			}
			if _, me := call(t, kc, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); !strings.Contains(me, `"max_rating":"PG"`) || !strings.Contains(me, `"perms":["ask"]`) {
				t.Fatalf("kid me: %s", me)
			}
			// A server that is down is not a wrong password: many tries never lock anyone out.
			for i := 0; i < 30; i++ {
				if _, code := login("down", "x"); code != 502 {
					t.Fatalf("server down try %d: %d", i, code)
				}
			}
			if _, code := login("mum", "right"); code != 200 {
				t.Fatalf("after the outage: %d", code)
			}
			// Any other odd answer still counts, so guessing stays limited. The
			// wrong password and the turned-off account above still count at
			// this address: a good sign-in takes back only its own try.
			for i := 0; i < freeTries-2; i++ {
				if _, code := login("boom", "x"); code != 502 {
					t.Fatalf("odd answer try %d: %d", i, code)
				}
			}
			if _, code := login("boom", "x"); code != 429 {
				t.Fatalf("odd answers were not counted: %d", code)
			}
			f.clock.add(MaxBackoff + time.Minute)
			login("mum", "right")
			// Old typos whose wait ran out: an outage try must not start the wait again.
			for i := 0; i < freeUserTries+1; i++ {
				login("typo", "x")
			}
			f.clock.add(MaxBackoff + time.Minute)
			for i := 0; i < 3; i++ {
				if _, code := login("down", "x"); code != 502 {
					t.Fatalf("outage try %d after old typos: %d", i, code)
				}
			}
			if logouts.Load() < 1 {
				t.Fatal("the server's session was not ended")
			}
			// A media-server person has no HolyArrs password.
			if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"Mum","password":""}`, nil); resp.StatusCode != 401 {
				t.Fatalf("empty password sign-in: %d", resp.StatusCode)
			}
			_, st := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
			if !strings.Contains(st, `"`+kind+`"`) {
				t.Fatalf("state ways: %s", st)
			}
		})
	}
}

// route: POST /api/v1/auth/password
// Someone who joined through a media server sets a first HolyArrs password,
// so they can still sign in while that server is down.
func TestMediaServerFirstPassword(t *testing.T) {
	var logouts atomic.Int32
	ms := fakeMediaServer(t, &logouts)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"`+ms.URL+`","join":"everyone"},"confirm_everyone":true}`)
	signIn := func(name string) person {
		t.Helper()
		c := client(t)
		resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/media-server/jellyfin", `{"name":"`+name+`","password":"right"}`, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("%s sign-in: %d %s", name, resp.StatusCode, out)
		}
		var v struct {
			CSRF string `json:"csrf"`
		}
		jsonOf(t, out, &v)
		return person{c, v.CSRF}
	}
	first := pw + "-first"
	mum := signIn("mum")
	if _, me := mum.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(me, `"has_password":false`) || !strings.Contains(me, `"outside_recent":true`) {
		t.Fatalf("me before: %s", me)
	}
	if resp, out := mum.do(t, f, "POST", "/api/v1/auth/password", `{"new":"`+first+`"}`); resp.StatusCode != 204 {
		t.Fatalf("first password: %d %s", resp.StatusCode, out)
	}
	if _, me := mum.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(me, `"has_password":true`) {
		t.Fatalf("me after: %s", me)
	}
	creds, _ := json.Marshal(map[string]string{"name": "Mum", "password": first})
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", string(creds), nil); resp.StatusCode != 200 {
		t.Fatalf("password sign-in: %d", resp.StatusCode)
	}
	// The Jellyfin sign-in they joined with still works.
	signIn("mum")
	// Changing a password that exists needs a local proof, not the media server.
	if resp, out := mum.do(t, f, "POST", "/api/v1/auth/password", `{"current":"`+first+`","new":"`+first+`-2"}`); resp.StatusCode != 403 || !strings.Contains(out, "reauth") {
		t.Fatalf("change through a media server session: %d %s", resp.StatusCode, out)
	}
	// An API key never changes its user's password.
	_, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"script","scopes":["admin"]}`)
	var k struct {
		Key string `json:"key"`
	}
	jsonOf(t, out, &k)
	if resp, _ := call(t, &http.Client{}, "POST", f.srv.URL+"/api/v1/auth/password", `{"current":"x","new":"`+first+`-3"}`, map[string]string{APIKeyHeader: k.Key}); resp.StatusCode != 403 {
		t.Fatalf("API key changed a password: %d", resp.StatusCode)
	}
	// A first password needs a fresh sign-in too: an older one could be a
	// stolen cookie, and a password would keep the account for good.
	kid := signIn("kid")
	f.clock.add(11 * time.Minute)
	if resp, out := kid.do(t, f, "POST", "/api/v1/auth/password", `{"new":"`+first+`"}`); resp.StatusCode != 403 || !strings.Contains(out, `"reauth"`) {
		t.Fatalf("first password from an old sign-in: %d %s", resp.StatusCode, out)
	}
	kid = signIn("kid")
	if resp, out := kid.do(t, f, "POST", "/api/v1/auth/password", `{"new":"`+first+`"}`); resp.StatusCode != 204 {
		t.Fatalf("first password from a fresh sign-in: %d %s", resp.StatusCode, out)
	}
}

func TestPlexEveryoneNeedsAServer(t *testing.T) {
	f := gated(t, Options{PlexURL: "http://127.0.0.1:1"})
	own := ownerOf(t, f)
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"join":"everyone"},"confirm_everyone":true}`); resp.StatusCode != 400 {
		t.Fatalf("every Plex account could join: %d", resp.StatusCode)
	}
}

func TestInviteDiesWithItsMaker(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	tok := invite(t, f, ad, `{}`)
	if resp, _ := own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ana")), `{"state":"disabled"}`); resp.StatusCode != 200 {
		t.Fatal("disable admin")
	}
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"kid","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("a turned-off admin's invite still worked: %d", resp.StatusCode)
	}
}

// route: POST /api/v1/auth/keys
// route: DELETE /api/v1/auth/keys/{id}
func TestMemberKeysAreTheirOwn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	if resp, _ := own.do(t, f, "PUT", fmt.Sprintf("/api/v1/users/%d", userID(t, f, "ana")), `{"perms":["settings"]}`); resp.StatusCode != 200 {
		t.Fatal("grant settings")
	}
	own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"owner key"}`)
	_, out := ad.do(t, f, "GET", "/api/v1/auth/keys", "")
	if strings.Contains(out, "owner key") {
		t.Fatalf("admin sees the owner's keys: %s", out)
	}
	if resp, _ := ad.do(t, f, "DELETE", "/api/v1/auth/keys/1", ""); resp.StatusCode != 404 {
		t.Fatalf("admin removed the owner's key: %d", resp.StatusCode)
	}
}

// route: POST /api/v1/auth/password
func TestChangePassword(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	other := client(t)
	call(t, other, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"nope","new":"another long one"}`); resp.StatusCode != 401 {
		t.Fatalf("wrong current: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"`+pw+`","new":"another long one"}`); resp.StatusCode != 204 {
		t.Fatalf("change: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 200 {
		t.Fatalf("this session ended: %d", resp.StatusCode)
	}
	if resp, _ := call(t, other, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("other session kept: %d", resp.StatusCode)
	}
}

func TestResetOwnerTurnsOffTwoFactor(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	turnOnTwoFactor(t, f, own)
	if err := ResetOwner(context.Background(), f.s, "luis", pw); err != nil {
		t.Fatal(err)
	}
	resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 200 || strings.Contains(out, "ticket") {
		t.Fatalf("after reset-owner: %d %s", resp.StatusCode, out)
	}
}
