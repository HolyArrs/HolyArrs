package accounts

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

type fakeOIDC struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	key     *rsa.PrivateKey
	kid     string
	nonce   string
	subject string
	aud     []string
	azp     string
	expiry  time.Time
	// B4 knobs: name in the ID token (nil = "Synthetic User"), iss override,
	// the userinfo answer's sub and name, and a token-endpoint error code.
	name     *string
	iss      string
	infoSub  string
	infoName string
	tokenErr string
}

func newFakeOIDC(t *testing.T) *fakeOIDC {
	t.Helper()
	f := &fakeOIDC{t: t, kid: "key-1", subject: "subject-1", aud: []string{"holyarrs"}, expiry: time.Now().Add(time.Hour)}
	f.rotate("key-1")
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.discovery)
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/token", f.token)
	mux.HandleFunc("/keys", f.keys)
	mux.HandleFunc("/userinfo", f.userinfo)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOIDC) options() OIDCOptions {
	return OIDCOptions{Issuer: f.srv.URL, ClientID: "holyarrs", ClientSecret: "<test-secret>",
		RedirectURL: "http://127.0.0.1:18745/api/v1/auth/oidc/callback", AllowLoopbackHTTP: true}
}

func (f *fakeOIDC) rotate(kid string) {
	f.t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.key, f.kid = key, kid
	f.mu.Unlock()
}

func (f *fakeOIDC) discovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	write := map[string]any{"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/auth",
		"token_endpoint": f.srv.URL + "/token", "jwks_uri": f.srv.URL + "/keys", "userinfo_endpoint": f.srv.URL + "/userinfo",
		"id_token_signing_alg_values_supported": []string{"RS256"}}
	_ = json.NewEncoder(w).Encode(write)
}

func (f *fakeOIDC) keys(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	key, kid := &f.key.PublicKey, f.kid
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: key, KeyID: kid, Algorithm: "RS256", Use: "sig"}}})
}

func (f *fakeOIDC) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.Form.Get("code_verifier") == "" || r.Form.Get("code") == "" {
		http.Error(w, "bad exchange", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	key, kid, nonce, subject, aud, azp, expiry := f.key, f.kid, f.nonce, f.subject, append([]string(nil), f.aud...), f.azp, f.expiry
	name, iss, tokenErr := "Synthetic User", f.srv.URL, f.tokenErr
	if f.name != nil {
		name = *f.name
	}
	if f.iss != "" {
		iss = f.iss
	}
	f.mu.Unlock()
	if tokenErr != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": tokenErr})
		return
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, &jose.SignerOptions{ExtraHeaders: map[jose.HeaderKey]any{jose.HeaderKey("kid"): kid}})
	if err != nil {
		f.t.Error(err)
		http.Error(w, "sign", http.StatusInternalServerError)
		return
	}
	claims := struct {
		Issuer  string           `json:"iss"`
		Subject string           `json:"sub"`
		Expiry  *jwt.NumericDate `json:"exp"`
		Issued  *jwt.NumericDate `json:"iat"`
		Nonce   string           `json:"nonce"`
		Name    string           `json:"name,omitempty"`
		AZP     string           `json:"azp,omitempty"`
		Aud     []string         `json:"aud"`
	}{Issuer: iss, Subject: subject, Expiry: jwt.NewNumericDate(expiry), Issued: jwt.NewNumericDate(time.Now()),
		Nonce: nonce, Name: name, AZP: azp, Aud: aud}
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		f.t.Error(err)
		http.Error(w, "sign", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "<discarded>", "token_type": "Bearer", "id_token": raw})
}

func TestOIDCRuntimeUsesPKCEAndVerifiesRotatedKeys(t *testing.T) {
	f := newFakeOIDC(t)
	r, err := newOIDCRuntime(f.options())
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	now := time.Now()
	authURL, browser, err := r.begin(context.Background(), now, "ip:127.0.0.1", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	q := u.Query()
	for _, key := range []string{"state", "nonce", "code_challenge"} {
		if q.Get(key) == "" {
			t.Fatalf("%s missing from authorization URL", key)
		}
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method = %q", q.Get("code_challenge_method"))
	}
	tx, err := r.takeTransaction(now, q["state"], browser)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.nonce = tx.nonce
	f.mu.Unlock()
	claims, err := r.exchange(context.Background(), "first", tx)
	if err != nil {
		t.Fatal(err)
	}
	if claims.subject != "subject-1" || claims.display != "Synthetic User" {
		t.Fatalf("claims = %#v", claims)
	}

	f.rotate("key-2")
	authURL, browser, err = r.begin(context.Background(), now, "ip:127.0.0.1", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	u, _ = url.Parse(authURL)
	tx, err = r.takeTransaction(now, u.Query()["state"], browser)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.nonce = tx.nonce
	f.mu.Unlock()
	if _, err := r.exchange(context.Background(), "second", tx); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
}

func TestOIDCRuntimeRejectsReplayNonceExpiryAndExtraAudience(t *testing.T) {
	f := newFakeOIDC(t)
	r, err := newOIDCRuntime(f.options())
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	now := time.Now()
	authURL, browser, err := r.begin(context.Background(), now, "ip:test", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	state := u.Query()["state"]
	tx, err := r.takeTransaction(now, state, browser)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.takeTransaction(now, state, browser); !errorsIs(err, errOIDCFlow) {
		t.Fatalf("replayed transaction error = %v", err)
	}

	tests := []struct {
		name  string
		nonce string
		aud   []string
		azp   string
		exp   time.Time
	}{
		{"nonce", "wrong", []string{"holyarrs"}, "", time.Now().Add(time.Hour)},
		{"extra audience missing azp", tx.nonce, []string{"holyarrs", "other"}, "", time.Now().Add(time.Hour)},
		{"wrong azp", tx.nonce, []string{"holyarrs"}, "other", time.Now().Add(time.Hour)},
		{"expired", tx.nonce, []string{"holyarrs"}, "", time.Now().Add(-time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.mu.Lock()
			f.nonce, f.aud, f.azp, f.expiry = tt.nonce, tt.aud, tt.azp, tt.exp
			f.mu.Unlock()
			if _, err := r.exchange(context.Background(), "bad", tx); err == nil {
				t.Fatal("token accepted")
			}
		})
	}
	f.mu.Lock()
	f.nonce, f.aud, f.azp, f.expiry = tx.nonce, []string{"holyarrs", "other"}, "holyarrs", time.Now().Add(time.Hour)
	f.mu.Unlock()
	if _, err := r.exchange(context.Background(), "multi-audience", tx); err != nil {
		t.Fatalf("multiple audiences with matching azp: %v", err)
	}
}

func TestOIDCRuntimeRequiresSameOriginEndpoints(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer other.Close()
	var issuer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/auth",
			"token_endpoint": other.URL + "/token", "jwks_uri": issuer + "/keys"})
	}))
	defer srv.Close()
	issuer = srv.URL
	r, err := newOIDCRuntime(OIDCOptions{Issuer: issuer, ClientID: "holyarrs", ClientSecret: "x",
		RedirectURL: "http://127.0.0.1:18745/api/v1/auth/oidc/callback", AllowLoopbackHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	_, _, err = r.begin(context.Background(), time.Now(), "ip:test", 0, "")
	if err == nil || !strings.Contains(err.Error(), "configuration") {
		t.Fatalf("error = %v", err)
	}
}

func TestOIDCRuntimeConfigurationIsOfflineAndAllowsPublicPKCE(t *testing.T) {
	r, err := newOIDCRuntime(OIDCOptions{Issuer: "https://unreachable.invalid/tenant", ClientID: "public-client",
		RedirectURL: "https://holyarrs.example.test/base/api/v1/auth/oidc/callback"})
	if err != nil {
		t.Fatal(err)
	}
	r.close()
}

func TestOIDCDiscoveryCancellationDoesNotPoisonRetry(t *testing.T) {
	provider := newFakeOIDC(t)
	r, err := newOIDCRuntime(provider.options())
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := r.begin(ctx, time.Now(), "ip:test", 0, ""); err == nil {
		t.Fatal("canceled discovery succeeded")
	}
	if _, _, err := r.begin(context.Background(), time.Now(), "ip:test", 0, ""); err != nil {
		t.Fatalf("retry after canceled discovery: %v", err)
	}
}

func TestOIDCDiscoveryOutageIsOneSharedCancelableAttempt(t *testing.T) {
	var calls atomic.Int32
	var issuer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-r.Context().Done()
	}))
	defer srv.Close()
	issuer = srv.URL
	r, err := newOIDCRuntime(OIDCOptions{Issuer: issuer, ClientID: "public-client",
		RedirectURL: "http://127.0.0.1:18745/api/v1/auth/oidc/callback", AllowLoopbackHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	const callers = 24
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			_, _, err := r.begin(ctx, time.Now(), "ip:one", 0, "")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errorsIs(err, errOIDCProvider) {
			t.Fatalf("discovery wait error = %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("discovery calls = %d, want 1", got)
	}
}

func TestOIDCCookieUsesConfiguredURLBase(t *testing.T) {
	redirect, _ := url.Parse("https://holyarrs.example.test/sso-test/api/v1/auth/oidc/callback")
	runtime := &oidcRuntime{redirect: redirect}
	rec := httptest.NewRecorder()
	(&Auth{}).setOIDCCookie(rec, runtime, "browser", 300)
	if got := rec.Header().Get("Set-Cookie"); !strings.Contains(got, "Path=/sso-test/") || !strings.Contains(got, "Secure") {
		t.Fatalf("OIDC cookie = %q", got)
	}
}

func TestOIDCResultsAreBoundedPerClient(t *testing.T) {
	r := &oidcRuntime{results: map[string]oidcResult{}}
	now := time.Now()
	for range oidcMaxPerIP {
		if _, err := r.saveResult(now, oidcClaims{subject: "sub", display: "name"}, oidcTransaction{ip: "ip:one"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.saveResult(now, oidcClaims{subject: "sub", display: "name"}, oidcTransaction{ip: "ip:one"}); !errorsIs(err, errOIDCFlow) {
		t.Fatalf("ninth result error = %v", err)
	}
	if _, err := r.saveResult(now, oidcClaims{subject: "sub", display: "name"}, oidcTransaction{ip: "ip:two"}); err != nil {
		t.Fatalf("other client result: %v", err)
	}
}

func TestOIDCRoutesConsumeCallbackAndFinishOnce(t *testing.T) {
	provider := newFakeOIDC(t)
	opt := provider.options()
	opt.AllowJoin = true
	f := gated(t, Options{SSO: SSOOptions{OIDC: &opt}})
	setupOwner(t, f, client(t))
	c := client(t)

	authURL, startCookie := startOIDC(t, f, c, "/api/v1/auth/oidc/start", "")
	u, _ := url.Parse(authURL)
	provider.mu.Lock()
	provider.nonce = u.Query().Get("nonce")
	provider.mu.Unlock()
	callback := f.srv.URL + "/api/v1/auth/oidc/callback?state=" + url.QueryEscape(u.Query().Get("state")) + "&code=accepted"
	resp, body := oidcCall(t, c, http.MethodGet, callback, "127.0.0.1:18745", "", nil)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login?sso=finish" {
		t.Fatalf("callback: %d %q %s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("callback privacy headers = %q %q", resp.Header.Get("Cache-Control"), resp.Header.Get("Referrer-Policy"))
	}

	resp, body = call(t, c, http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/finish", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"name":"Synthetic User"`) {
		t.Fatalf("finish: %d %s", resp.StatusCode, body)
	}
	resp, _ = call(t, c, http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/finish", "", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("finish replay = %d", resp.StatusCode)
	}
	replayReq, _ := http.NewRequest(http.MethodGet, callback, nil)
	replayReq.Host = "127.0.0.1:18745"
	replayReq.AddCookie(&http.Cookie{Name: oidcCookieName, Value: startCookie})
	replay, err := client(t).Do(replayReq)
	if err != nil {
		t.Fatal(err)
	}
	replay.Body.Close()
	if replay.StatusCode != http.StatusBadRequest {
		t.Fatalf("callback replay = %d", replay.StatusCode)
	}
}

func TestOIDCCallbackRejectsWrongHostAndProviderError(t *testing.T) {
	provider := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	c := client(t)
	authURL, startCookie := startOIDC(t, f, c, "/api/v1/auth/oidc/start", "")
	u, _ := url.Parse(authURL)
	state := url.QueryEscape(u.Query().Get("state"))
	callback := f.srv.URL + "/api/v1/auth/oidc/callback?state=" + state + "&code=accepted"
	resp, _ := oidcCall(t, c, http.MethodGet, callback, "wrong.example", "", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong callback host = %d", resp.StatusCode)
	}

	bad := f.srv.URL + "/api/v1/auth/oidc/callback?state=" + state + "&code=accepted&error=access_denied"
	req, _ := http.NewRequest(http.MethodGet, bad, nil)
	req.Host = "127.0.0.1:18745"
	req.AddCookie(&http.Cookie{Name: oidcCookieName, Value: startCookie})
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login?sso=failed" {
		t.Fatalf("code plus provider error = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	req, _ = http.NewRequest(http.MethodGet, callback, nil)
	req.Host = "127.0.0.1:18745"
	req.AddCookie(&http.Cookie{Name: oidcCookieName, Value: startCookie})
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("provider failure did not consume transaction: %d", resp.StatusCode)
	}
}

func TestOIDCLinkMutatesOnlyAtAuthenticatedFinish(t *testing.T) {
	provider := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	c := client(t)
	csrf := setupOwner(t, f, c)
	authURL, _ := startOIDC(t, f, c, "/api/v1/auth/oidc/link", csrf)
	u, _ := url.Parse(authURL)
	provider.mu.Lock()
	provider.nonce, provider.subject = u.Query().Get("nonce"), "link-subject"
	provider.mu.Unlock()
	callback := f.srv.URL + "/api/v1/auth/oidc/callback?state=" + url.QueryEscape(u.Query().Get("state")) + "&code=accepted"
	resp, body := oidcCall(t, c, http.MethodGet, callback, "127.0.0.1:18745", "", nil)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account?sso=finish" {
		t.Fatalf("link callback: %d %q %s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	var links int
	if err := f.s.Read().QueryRow("SELECT count(*) FROM sso_links").Scan(&links); err != nil || links != 0 {
		t.Fatalf("callback changed links: %d %v", links, err)
	}
	resp, body = call(t, c, http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/link-finish", "", map[string]string{CSRFHeader: csrf})
	if resp.StatusCode != http.StatusOK || body != "{\"linked\":true}\n" {
		t.Fatalf("link finish: %d %s", resp.StatusCode, body)
	}
	if err := f.s.Read().QueryRow("SELECT count(*) FROM sso_links WHERE kind = 'oidc' AND subject = 'link-subject'").Scan(&links); err != nil || links != 1 {
		t.Fatalf("link not saved: %d %v", links, err)
	}
}

func startOIDC(t *testing.T, f *fixture, c *http.Client, path, csrf string) (string, string) {
	t.Helper()
	headers := map[string]string{}
	if csrf != "" {
		headers[CSRFHeader] = csrf
	}
	resp, body := call(t, c, http.MethodPost, f.srv.URL+path, "", headers)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("OIDC start: %d %s", resp.StatusCode, body)
	}
	var out struct {
		AuthURL string `json:"auth_url"`
	}
	jsonOf(t, body, &out)
	u, _ := url.Parse(f.srv.URL)
	var browser string
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == oidcCookieName {
			browser = ck.Value
		}
	}
	if out.AuthURL == "" || browser == "" {
		t.Fatalf("OIDC start missing URL or cookie: %s", body)
	}
	return out.AuthURL, browser
}

func oidcCall(t *testing.T, c *http.Client, method, rawURL, host, body string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, rawURL, strings.NewReader(body))
	req.Host = host
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func pointer[T any](v T) *T { return &v }

func errorsIs(err, target error) bool {
	return err != nil && (err == target || strings.Contains(fmt.Sprint(err), target.Error()))
}
