package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ssoBody is a PUT /auth/sso body with the owner's password.
func ssoBody(t *testing.T, fields map[string]any) string {
	t.Helper()
	if _, ok := fields["password"]; !ok {
		fields["password"] = pw
	}
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// sqlDo writes straight to the store, to set up a case.
func sqlDo(t *testing.T, f *fixture, q string, args ...any) {
	t.Helper()
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

const linkedAt = "2026-01-01T00:00:00Z"

func ssoFixture(t *testing.T) (*fixture, person, *fakeOIDC) {
	t.Helper()
	idp := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{SavedLimits: OIDCLimits{AllowLoopbackHTTP: true}}})
	return f, ownerOf(t, f), idp
}

// route: GET /api/v1/auth/sso
// route: PUT /api/v1/auth/sso
// The owner sets up single sign-on in Settings; the secret is never shown.
func TestSSOSetUpFromSettings(t *testing.T) {
	f, own, idp := ssoFixture(t)
	secret := "client-" + fmt.Sprint(time.Now().UnixNano())
	save := map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "holyarrs", "client_secret": secret}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, map[string]any{"password": "not it", "on": true})); resp.StatusCode != 403 || !strings.Contains(out, "wrong_proof") {
		t.Fatalf("wrong password: %d %s", resp.StatusCode, out)
	}
	resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save))
	if resp.StatusCode != 200 || strings.Contains(out, secret) {
		t.Fatalf("save: %d %s", resp.StatusCode, out)
	}
	o := f.a.ssoNow().oidc
	if o == nil || o.opt.ClientSecret != secret || o.opt.Issuer != idp.srv.URL || !strings.HasSuffix(o.opt.RedirectURL, "/api/v1/auth/oidc/callback") {
		t.Fatalf("running setup: %+v", o)
	}
	// A typed callback on another host is never taken.
	save["redirect_url"] = "https://elsewhere.example/api/v1/auth/oidc/callback"
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save)); resp.StatusCode != 200 || strings.Contains(f.a.ssoNow().oidc.opt.RedirectURL, "elsewhere") {
		t.Fatalf("typed callback taken: %s", f.a.ssoNow().oidc.opt.RedirectURL)
	}
	delete(save, "redirect_url")
	save["client_secret"] = secret
	_, out = own.do(t, f, "GET", "/api/v1/auth/sso", "")
	if !strings.Contains(out, `"secret_set":true`) || strings.Contains(out, secret) || !strings.Contains(out, `"source":"settings"`) {
		t.Fatalf("view: %s", out)
	}
	var stored int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM settings WHERE value LIKE '%'||?||'%'", secret).Scan(&stored)
	if stored != 0 {
		t.Fatal("secret stored in the clear")
	}
	if _, st := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil); !strings.Contains(st, `"oidc"`) {
		t.Fatalf("sign-in page ways: %s", st)
	}
	// Letting everyone in needs confirming.
	save["allow_join"] = true
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save)); resp.StatusCode != 409 {
		t.Fatalf("join without confirming: %d", resp.StatusCode)
	}
	save["confirm_everyone"] = true
	delete(save, "client_secret") // kept: same issuer and client id
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save)); resp.StatusCode != 200 || f.a.ssoNow().oidc.opt.ClientSecret != secret || !f.a.ssoNow().oidc.opt.AllowJoin {
		t.Fatalf("join confirmed, secret kept: %d %s", resp.StatusCode, out)
	}
	// A restart starts it again from Settings.
	again := New(f.s, Options{Vault: f.a.vault, fastHash: true, SSO: SSOOptions{SavedLimits: OIDCLimits{AllowLoopbackHTTP: true}}})
	t.Cleanup(again.Close)
	if o := again.ssoNow().oidc; o == nil || o.opt.ClientSecret != secret {
		t.Fatal("saved single sign-on not started after a restart")
	}
	// Turning it off hides the button.
	save["on"] = false
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save)); resp.StatusCode != 200 || f.a.ssoNow().oidc != nil {
		t.Fatalf("turn off: %d", resp.StatusCode)
	}
}

// A saved secret never goes to a new issuer or client id, and moving to
// another provider removes the old links and ends single sign-on sessions.
func TestSSONewProviderNeedsTheSecretAgain(t *testing.T) {
	f, own, idp := ssoFixture(t)
	own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "holyarrs", "client_secret": "first-" + pw}))
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	sqlDo(t, f, "INSERT INTO sso_links(kind, issuer, subject, user_id, display_name, created_at) VALUES ('oidc', ?, 'sub-ana', ?, 'Ana', ?)", idp.srv.URL, userID(t, f, "ana"), linkedAt)
	sqlDo(t, f, "UPDATE sessions SET auth_method = 'oidc' WHERE user_id = ?", userID(t, f, "ana"))
	other := newFakeOIDC(t)
	for _, change := range []map[string]any{
		{"on": true, "issuer": other.srv.URL, "client_id": "holyarrs"},
		{"on": true, "issuer": idp.srv.URL, "client_id": "another-client"},
	} {
		if resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, change)); resp.StatusCode != 400 || !strings.Contains(out, "secret") {
			t.Fatalf("secret followed %v: %d %s", change, resp.StatusCode, out)
		}
	}
	if f.a.ssoNow().oidc.opt.Issuer != idp.srv.URL {
		t.Fatal("refused change still swapped the provider")
	}
	// A public client at the new issuer: no secret is sent at all.
	resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, map[string]any{"on": true, "issuer": other.srv.URL, "client_id": "holyarrs", "public_client": true}))
	if resp.StatusCode != 200 || f.a.ssoNow().oidc.opt.ClientSecret != "" {
		t.Fatalf("public client: %d %s", resp.StatusCode, out)
	}
	var links, secrets, sessions int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sso_links WHERE kind = 'oidc'").Scan(&links)
	_ = f.s.Read().QueryRow("SELECT count(*) FROM secrets WHERE kind = 'oidc_client_secret'").Scan(&secrets)
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sessions WHERE auth_method = 'oidc'").Scan(&sessions)
	if links != 0 || secrets != 0 || sessions != 0 || !strings.Contains(out, `"links_removed":1`) {
		t.Fatalf("after moving: links %d secrets %d sessions %d: %s", links, secrets, sessions, out)
	}
	if resp, _ := ana.do(t, f, "GET", "/api/v1/auth/me", ""); resp.StatusCode != 401 {
		t.Fatalf("single sign-on session survived the move: %d", resp.StatusCode)
	}
}

// Only the owner, with a fresh local proof, reaches the setup; never an API key
// or a member; and the environment, when set, owns it.
func TestSSOSettingsAreTheOwners(t *testing.T) {
	f, own, idp := ssoFixture(t)
	ana := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	for _, route := range [][2]string{{"GET", "/api/v1/auth/sso"}, {"PUT", "/api/v1/auth/sso"}, {"POST", "/api/v1/auth/sso/test"}, {"POST", "/api/v1/users/1/sso/unlink"}} {
		if resp, _ := ana.do(t, f, route[0], route[1], `{}`); resp.StatusCode != 403 {
			t.Fatalf("admin %s %s: %d", route[0], route[1], resp.StatusCode)
		}
	}
	_, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"script","scopes":["all"]}`)
	var k struct {
		Key string `json:"key"`
	}
	jsonOf(t, out, &k)
	for _, route := range [][2]string{{"GET", "/api/v1/auth/sso"}, {"PUT", "/api/v1/auth/sso"}, {"POST", "/api/v1/auth/sso/test"}, {"POST", "/api/v1/auth/sso/unlink"}} {
		if resp, _ := call(t, &http.Client{}, route[0], f.srv.URL+route[1], ssoBody(t, map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "x"}), map[string]string{APIKeyHeader: k.Key}); resp.StatusCode != 403 {
			t.Fatalf("API key %s %s: %d", route[0], route[1], resp.StatusCode)
		}
	}
	// Ten minutes on, the owner confirms the password again first (a password
	// sign-in; the first-run setup session stays recent for an hour).
	own = signIn(t, f, "luis")
	f.clock.add(11 * time.Minute)
	if resp, out := own.do(t, f, "GET", "/api/v1/auth/sso", ""); resp.StatusCode != 403 || !strings.Contains(out, "reauth") {
		t.Fatalf("stale owner session: %d %s", resp.StatusCode, out)
	}

	env := gated(t, Options{SSO: SSOOptions{OIDCFromEnv: true}})
	eo := ownerOf(t, env)
	if resp, _ := eo.do(t, env, "PUT", "/api/v1/auth/sso", ssoBody(t, map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "x"})); resp.StatusCode != 409 {
		t.Fatalf("environment-owned setup changed: %d", resp.StatusCode)
	}
	if _, out := eo.do(t, env, "GET", "/api/v1/auth/sso", ""); !strings.Contains(out, `"source":"environment"`) {
		t.Fatalf("environment view: %s", out)
	}
}

// route: POST /api/v1/auth/sso/test
// The test answers ok or one plain failure, and is rate limited.
func TestSSOTestIssuer(t *testing.T) {
	f, own, idp := ssoFixture(t)
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/sso/test", `{"issuer":"`+idp.srv.URL+`"}`); resp.StatusCode != 200 || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("good issuer: %d %s", resp.StatusCode, out)
	}
	resp, out := own.do(t, f, "POST", "/api/v1/auth/sso/test", `{"issuer":"`+idp.srv.URL+`/nothing-here"}`)
	if resp.StatusCode != 502 || out != `{"error":"no OpenID Connect provider answered at this issuer"}`+"\n" {
		t.Fatalf("bad issuer: %d %q", resp.StatusCode, out)
	}
	code := 0
	for i := 0; i < freeTries+2 && code != 429; i++ {
		r, _ := own.do(t, f, "POST", "/api/v1/auth/sso/test", `{"issuer":"`+idp.srv.URL+`"}`)
		code = r.StatusCode
	}
	if code != 429 {
		t.Fatal("tests were not rate limited")
	}
}

// route: GET /api/v1/auth/sso/links
// route: POST /api/v1/auth/sso/unlink
// route: POST /api/v1/users/{id}/sso/unlink
// Unlinking ends the sessions that identity made, and never leaves a person
// with no way in unless the owner confirms it.
func TestSSOUnlink(t *testing.T) {
	f, own, _ := ssoFixture(t)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	anaID := userID(t, f, "ana")
	link := func(user int64, subject string) {
		t.Helper()
		sqlDo(t, f, "INSERT INTO sso_links(kind, issuer, subject, user_id, display_name, created_at) VALUES ('oidc', 'https://idp.example.test', ?, ?, 'Someone', ?)", subject, user, linkedAt)
	}
	link(anaID, "sub-ana")
	_, out := ana.do(t, f, "GET", "/api/v1/auth/sso/links", "")
	if !strings.Contains(out, `"issuer":"https://idp.example.test"`) {
		t.Fatalf("my links: %s", out)
	}
	// A single sign-on session of ana's, and the browser session she uses now.
	sqlDo(t, f, "INSERT INTO sessions(hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, family, auth_method, local_auth_at) SELECT 'sso-session', user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, 'sso-family', 'oidc', '' FROM sessions WHERE user_id = ? LIMIT 1", anaID)
	body := `{"kind":"oidc","issuer":"https://idp.example.test"}`
	if resp, out := ana.do(t, f, "POST", "/api/v1/auth/sso/unlink", body); resp.StatusCode != 204 {
		t.Fatalf("unlink mine: %d %s", resp.StatusCode, out)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sessions WHERE hash = 'sso-session'").Scan(&n)
	if n != 0 {
		t.Fatal("the single sign-on session survived the unlink")
	}
	if resp, _ := ana.do(t, f, "GET", "/api/v1/auth/me", ""); resp.StatusCode != 200 {
		t.Fatal("the password session ended too")
	}
	if resp, _ := ana.do(t, f, "POST", "/api/v1/auth/sso/unlink", body); resp.StatusCode != 404 {
		t.Fatalf("unlink twice: %d", resp.StatusCode)
	}
	// Someone who only signs in through single sign-on keeps a way in.
	sqlDo(t, f, "UPDATE users SET password_hash = '' WHERE id = ?", anaID)
	link(anaID, "sub-ana")
	path := fmt.Sprintf("/api/v1/users/%d/sso/unlink", anaID)
	if resp, out := own.do(t, f, "POST", path, body); resp.StatusCode != 409 || !strings.Contains(out, "no_way_in") {
		t.Fatalf("owner unlinks the only way in: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", path, `{"kind":"oidc","issuer":"https://idp.example.test","confirm_no_way_in":true}`); resp.StatusCode != 204 {
		t.Fatalf("owner confirmed: %d %s", resp.StatusCode, out)
	}
	if resp, _ := ana.do(t, f, "POST", path, body); resp.StatusCode != 403 {
		t.Fatalf("a member unlinked someone: %d", resp.StatusCode)
	}
}

// A broken single sign-on setup turns off only single sign-on.
func TestBrokenOIDCKeepsTrustedProxy(t *testing.T) {
	f := setup(t, Options{SSO: SSOOptions{OIDC: &OIDCOptions{}, Forward: &ForwardAuthOptions{Issuer: "https://proxy.example.test", TrustedPeers: []string{"127.0.0.1/32"}}}})
	if f.a.ssoNow().oidc != nil || f.a.ssoNow().forward == nil {
		t.Fatal("a broken single sign-on setup turned off trusted proxy sign-in")
	}
}

// A secret saved before any issuer never goes to the first issuer typed later.
func TestSSOSecretWithoutIssuerNeverFollows(t *testing.T) {
	f, own, idp := ssoFixture(t)
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, map[string]any{"on": false, "client_secret": "early-" + pw})); resp.StatusCode != 200 {
		t.Fatalf("secret first: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "holyarrs"})); resp.StatusCode != 400 || !strings.Contains(out, "secret") {
		t.Fatalf("early secret followed the first issuer: %d %s", resp.StatusCode, out)
	}
	if f.a.ssoNow().oidc != nil {
		t.Fatal("refused save started single sign-on")
	}
}

// When the saved secret cannot be read after a save, single sign-on stops
// rather than keep the old setup running.
func TestSSOUnreadableSecretStopsSingleSignOn(t *testing.T) {
	f, own, idp := ssoFixture(t)
	save := map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "holyarrs", "client_secret": "kept-" + pw}
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save)); resp.StatusCode != 200 || f.a.ssoNow().oidc == nil {
		t.Fatal("first save")
	}
	sqlDo(t, f, "DELETE FROM secrets WHERE kind = 'oidc_client_secret'")
	delete(save, "client_secret")
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save)); resp.StatusCode != 500 {
		t.Fatalf("unreadable secret: %d", resp.StatusCode)
	}
	if f.a.ssoNow().oidc != nil {
		t.Fatal("the old setup kept running")
	}
}

// A sign-in already finishing when the owner turns single sign-on off
// cannot leave a session behind: the change waits for it, then ends it.
func TestSSOChangeWaitsForFinishingSignIns(t *testing.T) {
	f, own, idp := ssoFixture(t)
	save := map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "holyarrs", "client_secret": "fence-" + pw}
	own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save))
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	f.a.ssoMu.RLock() // a finish in flight
	done := make(chan int, 1)
	go func() {
		save["on"] = false
		resp, _ := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save))
		done <- resp.StatusCode
	}()
	select {
	case code := <-done:
		f.a.ssoMu.RUnlock()
		t.Fatalf("the change did not wait for the finish: %d", code)
	case <-time.After(300 * time.Millisecond):
	}
	// The finish makes its session, then lets go.
	sqlDo(t, f, "INSERT INTO sessions(hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, family, auth_method, local_auth_at) SELECT 'late-oidc', user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, 'late-family', 'oidc', '' FROM sessions WHERE user_id = ? LIMIT 1", userID(t, f, "ana"))
	f.a.ssoMu.RUnlock()
	if code := <-done; code != 200 {
		t.Fatalf("turn off: %d", code)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sessions WHERE hash = 'late-oidc'").Scan(&n)
	if n != 0 {
		t.Fatal("a session made while turning off survived")
	}
}

// A two-factor step left from a single sign-on sign-in ends with it: a
// change waits for one in flight, and one it dropped cannot finish later.
func TestSSOChangeEndsTwoFactorSteps(t *testing.T) {
	f, own, _ := ssoFixture(t)
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	put := func(tok string) {
		f.a.mu.Lock()
		f.a.tickets[sum(tok)] = &ticket{userID: userID(t, f, "ana"), name: "ana", method: "oidc", expires: f.a.now().Add(time.Minute)}
		f.a.mu.Unlock()
	}
	put("step-1")
	f.a.ssoMu.Lock() // a settings change in flight
	done := make(chan int, 1)
	go func() {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"step-1","code":"123456"}`, nil)
		done <- resp.StatusCode
	}()
	select {
	case code := <-done:
		f.a.ssoMu.Unlock()
		t.Fatalf("the two-factor step did not wait for the change: %d", code)
	case <-time.After(300 * time.Millisecond):
	}
	f.a.dropTickets(func(tk *ticket) bool { return tk.method == "oidc" })
	f.a.ssoMu.Unlock()
	if code := <-done; code != 401 {
		t.Fatalf("a dropped step finished: %d", code)
	}
}
