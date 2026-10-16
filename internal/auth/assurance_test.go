package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func setSessionAssurance(t *testing.T, f *fixture, query string, args ...any) {
	t.Helper()
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(query, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func meOf(t *testing.T, f *fixture, p person) Who {
	t.Helper()
	resp, out := p.do(t, f, http.MethodGet, "/api/v1/auth/me", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me: %d %s", resp.StatusCode, out)
	}
	var who Who
	if err := json.Unmarshal([]byte(out), &who); err != nil {
		t.Fatal(err)
	}
	return who
}

func TestLocalSessionAssuranceAndExpiry(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	who := meOf(t, f, own)
	if who.AuthMethod != AuthMethodLocalSetup || !who.LocalRecent {
		t.Fatalf("setup assurance = method %q recent %v", who.AuthMethod, who.LocalRecent)
	}

	f.clock.add(localRecentFor)
	who = meOf(t, f, own)
	if who.LocalRecent {
		t.Fatal("local proof stayed recent at its exact lifetime boundary")
	}

	setSessionAssurance(t, f, "UPDATE sessions SET local_auth_at = ?", storeTime(f.clock.t.Add(time.Minute)))
	who = meOf(t, f, own)
	if who.LocalRecent {
		t.Fatal("future local proof was accepted")
	}

	for label, update := range map[string]string{
		"malformed timestamp": "UPDATE sessions SET auth_method = 'local_password', local_auth_at = 'not-a-time'",
		"legacy method":       "UPDATE sessions SET auth_method = 'legacy', local_auth_at = '" + storeTime(f.clock.t) + "'",
	} {
		setSessionAssurance(t, f, update)
		if got := meOf(t, f, own); got.LocalRecent {
			t.Errorf("%s was accepted", label)
		}
	}
}

func TestPasswordLoginAndRotationPreserveLocalAssurance(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	p := signIn(t, f, "luis")
	who := meOf(t, f, p)
	if who.AuthMethod != AuthMethodLocalPassword || !who.LocalRecent {
		t.Fatalf("password assurance = method %q recent %v", who.AuthMethod, who.LocalRecent)
	}
	f.clock.add(RotateEvery + time.Minute)
	who = meOf(t, f, p)
	if who.AuthMethod != AuthMethodLocalPassword || who.LocalRecent {
		t.Fatalf("rotated assurance = method %q recent %v", who.AuthMethod, who.LocalRecent)
	}
}

func TestTwoFactorTicketPreservesOutsideAuthMethod(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	turnOnTwoFactor(t, f, own)
	id := userID(t, f, "luis")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/finish", nil)
	f.a.passedAs(rec, req, id, "luis", "oidc:test")
	if rec.Code != http.StatusOK {
		t.Fatalf("passedAs: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Ticket string `json:"ticket"`
	}
	jsonOf(t, rec.Body.String(), &out)
	f.a.mu.Lock()
	ticket := f.a.tickets[sum(out.Ticket)]
	f.a.mu.Unlock()
	if ticket == nil || ticket.method != "oidc:test" {
		t.Fatalf("ticket = %+v", ticket)
	}
}

func TestRecentLocalRequiresBrowserSessionAndCompletedTwoFactor(t *testing.T) {
	ctx := ContextWith(context.Background(), Who{Via: "session", LocalRecent: true})
	req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
	if !RecentLocal(req) {
		t.Fatal("recent local browser session refused")
	}
	for _, who := range []Who{
		{Via: "api_key", LocalRecent: true},
		{Via: "session", LocalRecent: false},
		{Via: "session", LocalRecent: true, MustTwoFactor: true},
	} {
		req = req.WithContext(ContextWith(context.Background(), who))
		if RecentLocal(req) {
			t.Fatalf("RecentLocal accepted %+v", who)
		}
	}
}

func TestGateRequiresRecentLocalForSessionAdminAndControl(t *testing.T) {
	mux := http.NewServeMux()
	for _, pattern := range []string{
		"GET /api/v1/titles",
		"GET /api/v1/auth/me",
		"POST /api/v1/auth/logout",
		"POST /api/v1/auth/reauth",
		"GET /api/v1/auth/sessions",
		"DELETE /api/v1/auth/sessions",
		"DELETE /api/v1/auth/sessions/{sid}",
		"GET /api/v1/auth/keys",
		"POST /api/v1/app/restart",
		"POST /api/v1/auth/two-factor/setup",
		"POST /api/v1/auth/two-factor/enable",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	}
	gate := Gate(mux)
	request := func(pattern string, who Who) int {
		method, path, _ := cutRoute(pattern)
		if path == "/api/v1/auth/sessions/{sid}" {
			path = "/api/v1/auth/sessions/one"
		}
		r := httptest.NewRequest(method, path, nil).WithContext(ContextWith(context.Background(), who))
		w := httptest.NewRecorder()
		gate.ServeHTTP(w, r)
		return w.Code
	}
	stale := Who{UserID: 1, Via: "session", Role: RoleOwner}
	for _, pattern := range []string{"GET /api/v1/auth/keys", "POST /api/v1/app/restart"} {
		if got := request(pattern, stale); got != http.StatusForbidden {
			t.Errorf("stale session %s = %d, want 403", pattern, got)
		}
	}
	for _, pattern := range []string{
		"GET /api/v1/titles", "GET /api/v1/auth/me", "POST /api/v1/auth/logout", "POST /api/v1/auth/reauth",
		"GET /api/v1/auth/sessions", "DELETE /api/v1/auth/sessions", "DELETE /api/v1/auth/sessions/{sid}",
	} {
		if got := request(pattern, stale); got != http.StatusTeapot {
			t.Errorf("recovery route %s = %d, want through", pattern, got)
		}
	}
	mustEnroll := stale
	mustEnroll.MustTwoFactor = true
	for _, pattern := range []string{"POST /api/v1/auth/two-factor/setup", "POST /api/v1/auth/two-factor/enable"} {
		if got := request(pattern, mustEnroll); got != http.StatusTeapot {
			t.Errorf("two-factor enrollment %s = %d, want through", pattern, got)
		}
	}
	fresh := stale
	fresh.LocalRecent = true
	for _, pattern := range []string{"GET /api/v1/auth/keys", "POST /api/v1/app/restart"} {
		if got := request(pattern, fresh); got != http.StatusTeapot {
			t.Errorf("recent local session %s = %d, want through", pattern, got)
		}
	}
	localSkip := stale
	localSkip.Via = "local_network"
	if got := request("POST /api/v1/app/restart", localSkip); got != http.StatusForbidden {
		t.Errorf("local-network bypass reached control route: %d", got)
	}
}

func TestReauthReplacesWholeFamilyWithFreshLocalSession(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	oldCookie := own.c.Jar.Cookies(mustURL(f.srv.URL))[0]
	var oldFamily string
	if err := f.s.Read().QueryRow("SELECT family FROM sessions WHERE hash = ?", sum(oldCookie.Value)).Scan(&oldFamily); err != nil {
		t.Fatal(err)
	}
	f.clock.add(RotateEvery + time.Minute)
	if resp, _ := own.do(t, f, http.MethodGet, "/api/v1/auth/me", ""); resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	setSessionAssurance(t, f, "UPDATE sessions SET auth_method = 'oidc:test', local_auth_at = '' WHERE family = ?", oldFamily)
	if resp, _ := own.do(t, f, http.MethodGet, "/api/v1/auth/keys", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("external session reached admin route: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, http.MethodPost, "/api/v1/auth/reauth", `{"password":"wrong"}`); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}
	resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/reauth", `{"password":"`+pw+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reauth: %d %s", resp.StatusCode, out)
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &session)
	own.csrf = session.CSRF
	if got := meOf(t, f, own); got.AuthMethod != AuthMethodLocalPassword || !got.LocalRecent {
		t.Fatalf("reauth assurance = method %q recent %v", got.AuthMethod, got.LocalRecent)
	}
	var left int
	if err := f.s.Read().QueryRow("SELECT count(*) FROM sessions WHERE family = ?", oldFamily).Scan(&left); err != nil || left != 0 {
		t.Fatalf("old family rows = %d, err %v", left, err)
	}
	old := client(t)
	old.Jar.SetCookies(mustURL(f.srv.URL), []*http.Cookie{oldCookie})
	if resp, _ := call(t, old, http.MethodGet, f.srv.URL+"/api/v1/auth/me", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old family predecessor works: %d", resp.StatusCode)
	}
}

func TestReauthRequiresEnrolledTwoFactorAndAcceptsBackupCode(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	_, codes := turnOnTwoFactor(t, f, own)
	setSessionAssurance(t, f, "UPDATE sessions SET auth_method = 'plex', local_auth_at = ''")
	if resp, _ := own.do(t, f, http.MethodPost, "/api/v1/auth/reauth", `{"password":"`+pw+`"}`); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reauth without second factor: %d", resp.StatusCode)
	}
	resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/reauth", `{"password":"`+pw+`","code":"`+codes[0]+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reauth with backup code: %d %s", resp.StatusCode, out)
	}
}

func TestLogoutDeletesWholeRotatedFamily(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	first := own.c.Jar.Cookies(mustURL(f.srv.URL))[0]
	f.clock.add(RotateEvery + time.Minute)
	if resp, _ := own.do(t, f, http.MethodGet, "/api/v1/auth/me", ""); resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	if resp, _ := own.do(t, f, http.MethodPost, "/api/v1/auth/logout", ""); resp.StatusCode != http.StatusNoContent {
		t.Fatal(resp.StatusCode)
	}
	old := client(t)
	old.Jar.SetCookies(mustURL(f.srv.URL), []*http.Cookie{first})
	if resp, _ := call(t, old, http.MethodGet, f.srv.URL+"/api/v1/auth/me", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("rotation predecessor survived logout: %d", resp.StatusCode)
	}
}

func storeTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }

func cutRoute(pattern string) (string, string, bool) {
	for i, r := range pattern {
		if r == ' ' {
			return pattern[:i], pattern[i+1:], true
		}
	}
	return pattern, "", false
}
