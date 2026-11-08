package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
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

func meOf(t *testing.T, f *fixture, p person) webserver.Caller {
	t.Helper()
	resp, out := p.do(t, f, http.MethodGet, "/api/v1/auth/me", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me: %d %s", resp.StatusCode, out)
	}
	var who webserver.Caller
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
	ctx := webserver.ContextWith(context.Background(), webserver.Caller{Via: "session", LocalRecent: true})
	req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
	if !RecentLocal(req) {
		t.Fatal("recent local browser session refused")
	}
	for _, who := range []webserver.Caller{
		{Via: "api_key", LocalRecent: true},
		{Via: "session", LocalRecent: false},
		{Via: "session", LocalRecent: true, MustTwoFactor: true},
	} {
		req = req.WithContext(webserver.ContextWith(context.Background(), who))
		if RecentLocal(req) {
			t.Fatalf("RecentLocal accepted %+v", who)
		}
	}
}

// gateCode sends one request for pattern through Gate, signed in as who.
func gateCode(pattern string, who webserver.Caller) (int, string) {
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	method, path, _ := cutRoute(pattern)
	path = regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(path, "1")
	r := httptest.NewRequest(method, path, nil).WithContext(webserver.ContextWith(context.Background(), who))
	w := httptest.NewRecorder()
	Gate(mux).ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

// A browser session is asked for its password again only
// for risky actions; opening any page or saving an ordinary setting never.
func TestGateAsksStaleBrowserSessionOnlyOnRiskyRoutes(t *testing.T) {
	var all []string
	for p := range riskyRoutes {
		if _, ok := RoutePerms[p]; !ok {
			t.Errorf("risky route %q is not a known route", p)
		}
		all = append(all, p)
	}
	// The whole list, so a route cannot drop off it unnoticed.
	sort.Strings(all)
	if want := []string{
		"DELETE /api/v1/auth/keys/{id}", "DELETE /api/v1/auth/plex/link", "DELETE /api/v1/auth/plex/my-link", "DELETE /api/v1/backups/{name}",
		"DELETE /api/v1/settings/old-app-api/keys/{side}", "GET /api/v1/auth/sso", "GET /api/v1/backups/{name}",
		"POST /api/v1/auth/ai-connect", "POST /api/v1/auth/forward/link", "POST /api/v1/auth/import/{kind}", "POST /api/v1/auth/keys",
		"POST /api/v1/auth/oidc/link", "POST /api/v1/auth/oidc/link-finish", "POST /api/v1/auth/password",
		"POST /api/v1/auth/plex/link/finish", "POST /api/v1/auth/plex/link/start", "POST /api/v1/auth/plex/my-link/finish", "POST /api/v1/auth/plex/my-link/start",
		"POST /api/v1/auth/sso/test", "POST /api/v1/auth/sso/unlink",
		"POST /api/v1/auth/two-factor/backup-codes", "POST /api/v1/auth/two-factor/disable", "POST /api/v1/auth/two-factor/enable", "POST /api/v1/auth/two-factor/setup",
		"POST /api/v1/backups/restore", "POST /api/v1/migrate/moves", "POST /api/v1/settings/old-app-api/keys/{side}", "POST /api/v1/system/key-kit", "POST /api/v1/users/invites",
		"POST /api/v1/users/{id}/password", "POST /api/v1/users/{id}/sso/unlink", "POST /api/v1/users/{id}/two-factor/reset",
		"PUT /api/v1/auth/settings", "PUT /api/v1/auth/sso", "PUT /api/v1/users/{id}",
	}; !slices.Equal(all, want) {
		t.Errorf("risky routes = %q", all)
	}
	stale := webserver.Caller{UserID: 1, Via: "session", Role: webserver.RoleOwner, Perms: webserver.DefaultPerms(webserver.RoleOwner)}
	fresh := stale
	fresh.LocalRecent = true
	var riskyGets []string
	for p := range RoutePerms {
		code, body := gateCode(p, stale)
		if riskyRoutes[p] && !recentLocalExempt(p, stale) {
			if code != http.StatusForbidden || !strings.Contains(body, `"reauth"`) {
				t.Errorf("stale session reached risky %s: %d %s", p, code, body)
			}
			if strings.HasPrefix(p, "GET ") {
				riskyGets = append(riskyGets, p)
			}
		} else if code != http.StatusTeapot {
			t.Errorf("stale session asked again on %s: %d %s", p, code, body)
		}
		if code, body := gateCode(p, fresh); code != http.StatusTeapot {
			t.Errorf("recent session refused on %s: %d %s", p, code, body)
		}
	}
	// Only two pages ask: the backup file is the whole database, and single
	// sign-on setup is the owner's sign-in door.
	sort.Strings(riskyGets)
	if want := []string{"GET /api/v1/auth/sso", "GET /api/v1/backups/{name}"}; !slices.Equal(riskyGets, want) {
		t.Errorf("risky pages = %v, want %v", riskyGets, want)
	}
	for _, p := range []string{
		"GET /api/v1/auth/keys", "GET /api/v1/users", "GET /api/v1/logs", "GET /api/v1/backups", "GET /api/v1/auth/settings",
		"GET /api/v1/settings/ai", "GET /api/v1/app/storage", "PUT /api/v1/settings/import-folders", "POST /api/v1/app/restart",
	} {
		if code, _ := gateCode(p, stale); code != http.StatusTeapot {
			t.Errorf("ordinary %s asked again: %d", p, code)
		}
	}
	for _, p := range []string{
		"POST /api/v1/auth/two-factor/disable", "POST /api/v1/auth/keys", "DELETE /api/v1/auth/keys/{id}",
		"PUT /api/v1/users/{id}", "POST /api/v1/backups/restore", "PUT /api/v1/auth/settings", "PUT /api/v1/auth/sso",
	} {
		if code, _ := gateCode(p, stale); code != http.StatusForbidden {
			t.Errorf("risky %s went through: %d", p, code)
		}
	}
}

func TestGateRecoveryAndNonSessionSignIns(t *testing.T) {
	stale := webserver.Caller{UserID: 1, Via: "session", Role: webserver.RoleOwner, Perms: webserver.DefaultPerms(webserver.RoleOwner)}
	for _, p := range []string{
		"GET /api/v1/titles", "GET /api/v1/auth/me", "POST /api/v1/auth/logout", "POST /api/v1/auth/reauth",
		"GET /api/v1/auth/sessions", "DELETE /api/v1/auth/sessions", "DELETE /api/v1/auth/sessions/{sid}",
	} {
		if code, _ := gateCode(p, stale); code != http.StatusTeapot {
			t.Errorf("recovery route %s = %d, want through", p, code)
		}
	}
	mustEnroll := stale
	mustEnroll.MustTwoFactor = true
	for _, p := range []string{"POST /api/v1/auth/two-factor/setup", "POST /api/v1/auth/two-factor/enable"} {
		if code, _ := gateCode(p, mustEnroll); code != http.StatusTeapot {
			t.Errorf("two-factor enrollment %s = %d, want through", p, code)
		}
	}
	// The trusted-network skip signs in without any password, so it keeps
	// off every admin and control route, as before.
	localSkip := stale
	localSkip.Via = "local_network"
	for p := range RoutePerms {
		sc := RouteScope(p)
		code, _ := gateCode(p, localSkip)
		if (sc == webserver.ScopeAdmin || sc == webserver.ScopeControl) && !recentLocalExempt(p, localSkip) {
			if code != http.StatusForbidden {
				t.Errorf("local-network skip reached %s: %d", p, code)
			}
		} else if code == http.StatusForbidden {
			t.Errorf("local-network skip refused ordinary %s", p)
		}
	}
}

// A real sign-in whose proof went stale opens pages and keeps working;
// only the risky action asks, and one confirmation lets it through.
func TestStaleSessionOpensPagesAndAsksOnlyForRiskyAction(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	// A password sign-in: the first-run setup session stays recent longer.
	own := signIn(t, f, "luis")
	f.clock.add(localRecentFor + time.Minute)
	if who := meOf(t, f, own); who.LocalRecent {
		t.Fatal("proof still recent")
	}
	for _, path := range []string{"/api/v1/auth/keys", "/api/v1/users", "/api/v1/auth/settings", "/api/v1/users/invites"} {
		if resp, out := own.do(t, f, http.MethodGet, path, ""); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s with a stale proof: %d %s", path, resp.StatusCode, out)
		}
	}
	resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/keys", `{"label":"script"}`)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(out, `"reauth"`) {
		t.Fatalf("new API key with a stale proof: %d %s", resp.StatusCode, out)
	}
	resp, out = own.do(t, f, http.MethodPost, "/api/v1/auth/reauth", `{"password":"`+pw+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reauth: %d %s", resp.StatusCode, out)
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &session)
	own.csrf = session.CSRF
	if resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/keys", `{"label":"script"}`); resp.StatusCode/100 != 2 {
		t.Fatalf("new API key after confirming: %d %s", resp.StatusCode, out)
	}
	f.clock.add(localRecentFor)
	if resp, _ := own.do(t, f, http.MethodPost, "/api/v1/auth/keys", `{"label":"later"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("confirmation outlived its 10 minutes: %d", resp.StatusCode)
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
	setSessionAssurance(t, f, "UPDATE sessions SET auth_method = 'oidc:test', local_auth_at = '', family = ? WHERE family = ?", strings.TrimPrefix(oldFamily, setupFamily), oldFamily)
	oldFamily = strings.TrimPrefix(oldFamily, setupFamily) // no longer the setup session
	if resp, _ := own.do(t, f, http.MethodPost, "/api/v1/auth/keys", `{"label":"x"}`); resp.StatusCode != http.StatusForbidden {
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
