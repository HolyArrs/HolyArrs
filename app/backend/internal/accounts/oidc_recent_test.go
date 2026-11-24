package accounts

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// oidcOwner is a no-password owner (made through Jellyfin) whose OIDC
// identity is linked, with the provider that signs them in.
func oidcOwner(t *testing.T) (*fixture, *fakeOIDC) {
	t.Helper()
	provider := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	jellyfinOwner(t, f)
	sqlDo(t, f, "INSERT INTO sso_links(kind, issuer, subject, user_id, display_name, created_at) VALUES ('oidc', ?, 'subject-1', ?, 'Boss', '2026-01-01T00:00:00Z')",
		provider.srv.URL, ownerID(t, f))
	return f, provider
}

// oidcIn signs in through the provider. fresh asks for a real new sign-in;
// authTime is the ID token's auth_time (nil leaves it out).
func oidcIn(t *testing.T, f *fixture, provider *fakeOIDC, fresh bool, authTime *time.Time) person {
	t.Helper()
	c := client(t)
	body := ""
	if fresh {
		body = `{"fresh":true}`
	}
	resp, out := call(t, c, http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/start", body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start: %d %s", resp.StatusCode, out)
	}
	var start struct {
		AuthURL string `json:"auth_url"`
	}
	jsonOf(t, out, &start)
	u, _ := url.Parse(start.AuthURL)
	q := u.Query()
	if fresh != (q.Get("prompt") == "login" && q.Get("max_age") == "0") {
		t.Fatalf("fresh=%v but prompt=%q max_age=%q", fresh, q.Get("prompt"), q.Get("max_age"))
	}
	provider.mu.Lock()
	provider.nonce, provider.authTime = q.Get("nonce"), authTime
	provider.mu.Unlock()
	callback := f.srv.URL + "/api/v1/auth/oidc/callback?state=" + url.QueryEscape(q.Get("state")) + "&code=accepted"
	if resp, out := oidcCall(t, c, http.MethodGet, callback, "127.0.0.1:18745", "", nil); resp.StatusCode != http.StatusSeeOther ||
		resp.Header.Get("Location") != "/login?sso=finish" {
		t.Fatalf("callback: %d %q %s", resp.StatusCode, resp.Header.Get("Location"), out)
	}
	resp, out = call(t, c, http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/finish", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("finish: %d %s", resp.StatusCode, out)
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &v)
	return person{c, v.CSRF}
}

// A no-password owner who signs in again through OIDC with a real new
// sign-in at the provider (auth_time just now) may make protected changes
// and set a first password, for ten minutes.
func TestOIDCFreshSignInCountsForNoPasswordOwner(t *testing.T) {
	f, provider := oidcOwner(t)
	at := f.clock.now().Add(-time.Minute)
	p := oidcIn(t, f, provider, true, &at)
	if who := meOf(t, f, p); !who.OutsideRecent || who.LocalRecent {
		t.Fatalf("fresh OIDC: %+v", who)
	}
	if !signInSettings(t, f, p) {
		t.Fatal("a fresh OIDC sign-in of a no-password owner was refused")
	}
	if resp, out := p.do(t, f, http.MethodPost, "/api/v1/auth/password", `{"new":"`+pw+`"}`); resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first password: %d %s", resp.StatusCode, out)
	}

	f2, provider2 := oidcOwner(t)
	at2 := f2.clock.now()
	stale := oidcIn(t, f2, provider2, true, &at2)
	f2.clock.add(localRecentFor + time.Minute)
	if signInSettings(t, f2, stale) {
		t.Fatal("an OIDC sign-in older than ten minutes still passed")
	}
}

// The provider's own sign-in time decides, not when HolyArrs got the answer.
func TestOIDCSignInWithoutRealRecheckRefused(t *testing.T) {
	cases := []struct {
		name  string
		fresh bool
		age   *time.Duration
	}{
		{"auth_time too old", true, pointer(localRecentFor + time.Minute)},
		{"auth_time missing", true, nil},
		{"auth_time far ahead", true, pointer(-time.Hour)},
		{"no re-check asked", false, pointer(time.Duration(0))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, provider := oidcOwner(t)
			var at *time.Time
			if tc.age != nil {
				at = pointer(f.clock.now().Add(-*tc.age))
			}
			p := oidcIn(t, f, provider, tc.fresh, at)
			if signInSettings(t, f, p) {
				t.Fatalf("%s: OIDC sign-in counted as recent proof", tc.name)
			}
			if resp, out := p.do(t, f, http.MethodPost, "/api/v1/auth/password", `{"new":"`+pw+`"}`); resp.StatusCode != http.StatusForbidden {
				t.Fatalf("%s: first password set: %d %s", tc.name, resp.StatusCode, out)
			}
		})
	}
}

// A trusted-proxy owner with no password sets a first password with the
// owner reset code from the server log.
func TestForwardAuthOwnerSetsPasswordWithResetCode(t *testing.T) {
	f := gated(t, Options{})
	_, signIn, _ := jellyfinOwner(t, f)
	p := signIn("boss")
	setSessionAssurance(t, f, "UPDATE sessions SET auth_method = 'forward_auth' WHERE user_id = ?", ownerID(t, f))
	if resp, out := p.do(t, f, http.MethodPost, "/api/v1/auth/password", `{"new":"`+pw+`"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("proxy session set the first password: %d %s", resp.StatusCode, out)
	}
	code, err := MakeOwnerResetCode(t.Context(), f.s, ResetCodeLife, f.clock.now())
	if err != nil {
		t.Fatal(err)
	}
	if st, out := finishReset(t, f, code, "boss", pw); st != http.StatusOK {
		t.Fatalf("reset finish: %d %s", st, out)
	}
	if has, err := hasPassword(t.Context(), f.s.Read(), ownerID(t, f)); err != nil || !has {
		t.Fatalf("no password after reset: %v %v", has, err)
	}
}
