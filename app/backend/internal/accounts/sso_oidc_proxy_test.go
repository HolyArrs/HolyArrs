package accounts

// Plan B4 (single sign-on proof): the gaps the live Authelia run found or
// could not reach with a real provider.

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func (f *fakeOIDC) userinfo(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	sub, name := f.infoSub, f.infoName
	f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer <discarded>" || sub == "" {
		http.Error(w, "no", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"sub": sub, "name": name})
}

// b4Exchange runs one begin/take/exchange against the fake provider.
func b4Exchange(t *testing.T, f *fakeOIDC, r *oidcRuntime, code string) (oidcClaims, error) {
	t.Helper()
	authURL, browser, err := r.begin(context.Background(), time.Now(), "ip:test", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	tx, err := r.takeTransaction(time.Now(), u.Query()["state"], browser)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.nonce = tx.nonce
	f.mu.Unlock()
	return r.exchange(context.Background(), code, tx)
}

// Authelia 4.39 sends profile claims only from userinfo: the name comes from
// there, and only when userinfo answers for the same subject.
func TestOIDCNameFromUserInfoWhenIDTokenHasNone(t *testing.T) {
	f := newFakeOIDC(t)
	empty := ""
	f.name, f.infoSub, f.infoName = &empty, "subject-1", "Alice Lab"
	r, err := newOIDCRuntime(f.options())
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	claims, err := b4Exchange(t, f, r, "c1")
	if err != nil || claims.display != "Alice Lab" {
		t.Fatalf("userinfo name: %#v %v", claims, err)
	}
	f.mu.Lock()
	f.infoSub = "someone-else"
	f.mu.Unlock()
	claims, err = b4Exchange(t, f, r, "c2")
	if err != nil || claims.display != "oidc-user" || claims.subject != "subject-1" {
		t.Fatalf("userinfo for another subject must not name this one: %#v %v", claims, err)
	}
	// A name in the ID token wins; userinfo is not asked.
	named := "From Token"
	f.mu.Lock()
	f.name, f.infoSub = &named, "subject-1"
	f.mu.Unlock()
	if claims, err = b4Exchange(t, f, r, "c3"); err != nil || claims.display != "From Token" {
		t.Fatalf("token name: %#v %v", claims, err)
	}
}

// The callback only accepts ID tokens from the configured issuer, and a wrong
// client secret logs a plain hint without the secret or the code.
func TestOIDCWrongIssuerAndWrongSecret(t *testing.T) {
	f := newFakeOIDC(t)
	r, err := newOIDCRuntime(f.options())
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	f.iss = "https://other-provider.example"
	if _, err := b4Exchange(t, f, r, "c1"); err == nil {
		t.Fatal("ID token from another issuer accepted")
	}
	f.mu.Lock()
	f.iss, f.tokenErr = "", "invalid_client"
	f.mu.Unlock()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	_, err = b4Exchange(t, f, r, "code-must-not-be-logged")
	log.SetOutput(prev)
	if err == nil {
		t.Fatal("refused exchange accepted")
	}
	out := buf.String()
	if !strings.Contains(out, "client id or secret") || strings.Contains(out, "code-must-not-be-logged") || strings.Contains(out, "<test-secret>") {
		t.Fatalf("log = %q", out)
	}
}

// One refused sign-in must not fail the next person's last steps: only the
// start of a sign-in counts toward the address's limit.
func TestSSOOnlyTheStartCounts(t *testing.T) {
	a := &Auth{fails: map[string]*failure{}, now: time.Now}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:5000"
	for i := 0; i < freeTries-1; i++ {
		if !a.ssoAttempt(httptest.NewRecorder(), req) {
			t.Fatalf("start %d refused", i)
		}
	}
	for i := 0; i < 10; i++ {
		if !a.ssoContinue(httptest.NewRecorder(), req) {
			t.Fatalf("callback/finish %d refused", i)
		}
	}
	if !a.ssoAttempt(httptest.NewRecorder(), req) {
		t.Fatal("last free start refused")
	}
	w := httptest.NewRecorder()
	if a.ssoAttempt(w, req) || w.Code != http.StatusTooManyRequests {
		t.Fatalf("start past the limit = %d", w.Code)
	}
}

// A provider failure after the browser comes back lands on the sign-in page,
// not a bare error with the code in the address bar.
func TestOIDCCallbackFailureGoesToSignInPage(t *testing.T) {
	provider := newFakeOIDC(t)
	provider.tokenErr = "invalid_client"
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	c := client(t)
	authURL, _ := startOIDC(t, f, c, "/api/v1/auth/oidc/start", "")
	u, _ := url.Parse(authURL)
	callback := f.srv.URL + "/api/v1/auth/oidc/callback?state=" + url.QueryEscape(u.Query().Get("state")) + "&code=accepted"
	resp, body := oidcCall(t, c, http.MethodGet, callback, "127.0.0.1:18745", "", nil)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login?sso=failed" {
		t.Fatalf("failed exchange: %d %q %s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	resp, _ = call(t, c, http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/finish", "", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("finish after a failed callback = %d", resp.StatusCode)
	}
}
