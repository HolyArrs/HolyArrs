package accounts

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	oidcCookieName = "holyarrs_oidc"
	oidcFlowLife   = 5 * time.Minute
	oidcMaxFlows   = 1024
	oidcMaxPerIP   = 8
)

var (
	errOIDCConfig   = errors.New("OIDC configuration is invalid")
	errOIDCFlow     = errors.New("OIDC sign-in could not be continued")
	errOIDCProvider = errors.New("OIDC provider did not accept sign-in")
)

// OIDCOptions configures one fixed OpenID Connect provider.
type OIDCOptions struct {
	Issuer            string
	ClientID          string
	ClientSecret      string
	RedirectURL       string
	AllowJoin         bool
	AllowLocal        bool
	AllowLoopbackHTTP bool
}

type oidcRuntime struct {
	opt      OIDCOptions
	issuer   *url.URL
	redirect *url.URL
	client   *http.Client
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
	userinfo string // same-origin userinfo endpoint, asked only when the ID token has no name
	ctx      context.Context
	cancel   context.CancelFunc
	provider chan struct{}

	mu                sync.Mutex
	discovering       chan struct{}
	discoveryErr      error
	discoveryErrUntil time.Time
	flows             map[string]oidcTransaction
	results           map[string]oidcResult
}

type oidcTransaction struct {
	cookie  string
	nonce   string
	pkce    string
	ip      string
	expires time.Time
	userID  int64
	family  string
}

type oidcClaims struct {
	subject string
	display string
}

type oidcResult struct {
	oidcClaims
	expires time.Time
	ip      string
	userID  int64
	family  string
}

func newOIDCRuntime(opt OIDCOptions) (*oidcRuntime, error) {
	issuer, redirect, err := validateOIDCOptions(opt)
	if err != nil {
		return nil, errOIDCConfig
	}
	client := newOIDCHTTPClient(opt, issuer)
	ctx, cancel := context.WithCancel(oidc.ClientContext(context.Background(), client))
	r := &oidcRuntime{
		opt: opt, issuer: issuer, redirect: redirect, client: client, ctx: ctx, cancel: cancel,
		provider: make(chan struct{}, 4), flows: map[string]oidcTransaction{}, results: map[string]oidcResult{},
	}
	return r, nil
}

func (r *oidcRuntime) ensureProvider(ctx context.Context) error {
	r.mu.Lock()
	if r.oauth != nil && r.verifier != nil {
		r.mu.Unlock()
		return nil
	}
	if r.discoveryErr != nil && time.Now().Before(r.discoveryErrUntil) {
		err := r.discoveryErr
		r.mu.Unlock()
		return err
	}
	if r.discovering == nil {
		r.discovering = make(chan struct{})
		go r.discoverProvider(r.discovering)
	}
	done := r.discovering
	r.mu.Unlock()
	select {
	case <-ctx.Done():
		return errOIDCProvider
	case <-done:
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.oauth != nil && r.verifier != nil {
			return nil
		}
		return r.discoveryErr
	}
}

func (r *oidcRuntime) discoverProvider(done chan struct{}) {
	discoveryCtx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()
	select {
	case r.provider <- struct{}{}:
		defer func() { <-r.provider }()
	case <-discoveryCtx.Done():
		r.finishDiscovery(done, errOIDCProvider, nil, nil)
		return
	}
	discoveryCtx = oidc.ClientContext(discoveryCtx, r.client)
	provider, err := oidc.NewProvider(discoveryCtx, r.opt.Issuer)
	if err != nil {
		r.finishDiscovery(done, errOIDCProvider, nil, nil)
		return
	}
	var metadata struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		JWKSURI               string `json:"jwks_uri"`
		UserInfo              string `json:"userinfo_endpoint"`
	}
	if provider.Claims(&metadata) != nil || metadata.Issuer != r.opt.Issuer ||
		!oidcEndpointAllowed(r.issuer, metadata.AuthorizationEndpoint) ||
		!oidcEndpointAllowed(r.issuer, metadata.TokenEndpoint) ||
		!oidcEndpointAllowed(r.issuer, metadata.JWKSURI) {
		r.finishDiscovery(done, errOIDCConfig, nil, nil)
		return
	}
	oauth := &oauth2.Config{ClientID: r.opt.ClientID, ClientSecret: r.opt.ClientSecret, Endpoint: provider.Endpoint(),
		RedirectURL: r.opt.RedirectURL, Scopes: []string{oidc.ScopeOpenID, "profile"}}
	if r.opt.ClientSecret == "" {
		oauth.Endpoint.AuthStyle = oauth2.AuthStyleInParams
	} else {
		oauth.Endpoint.AuthStyle = oauth2.AuthStyleInHeader
	}
	verifier := provider.VerifierContext(r.ctx, &oidc.Config{ClientID: r.opt.ClientID, SupportedSigningAlgs: []string{oidc.RS256}})
	if oidcEndpointAllowed(r.issuer, metadata.UserInfo) {
		r.mu.Lock()
		r.userinfo = metadata.UserInfo
		r.mu.Unlock()
	}
	r.finishDiscovery(done, nil, oauth, verifier)
}

func (r *oidcRuntime) finishDiscovery(done chan struct{}, err error, oauth *oauth2.Config, verifier *oidc.IDTokenVerifier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.discovering != done {
		return
	}
	r.oauth, r.verifier, r.discoveryErr = oauth, verifier, err
	if err != nil {
		r.discoveryErrUntil = time.Now().Add(time.Second)
	} else {
		r.discoveryErrUntil = time.Time{}
	}
	close(done)
	r.discovering = nil
}

func validateOIDCOptions(opt OIDCOptions) (*url.URL, *url.URL, error) {
	if strings.TrimSpace(opt.Issuer) != opt.Issuer || strings.TrimSpace(opt.ClientID) == "" || opt.ClientID != strings.TrimSpace(opt.ClientID) ||
		strings.TrimSpace(opt.RedirectURL) != opt.RedirectURL || len(opt.Issuer) > 1000 || len(opt.ClientID) > 1000 ||
		len(opt.ClientSecret) > 4096 || len(opt.RedirectURL) > 2000 {
		return nil, nil, errOIDCConfig
	}
	issuer, err := url.Parse(opt.Issuer)
	if err != nil || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" {
		return nil, nil, errOIDCConfig
	}
	redirect, err := url.Parse(opt.RedirectURL)
	if err != nil || redirect.Host == "" || redirect.User != nil || redirect.RawQuery != "" || redirect.Fragment != "" || redirect.RawPath != "" ||
		!strings.HasSuffix(redirect.Path, "/api/v1/auth/oidc/callback") {
		return nil, nil, errOIDCConfig
	}
	base := strings.TrimSuffix(redirect.Path, "/api/v1/auth/oidc/callback")
	cleanBase, err := webserver.CleanURLBase(base)
	if err != nil || cleanBase != base {
		return nil, nil, errOIDCConfig
	}
	if !oidcURLSchemeAllowed(issuer, opt.AllowLoopbackHTTP) || !oidcURLSchemeAllowed(redirect, opt.AllowLoopbackHTTP) {
		return nil, nil, errOIDCConfig
	}
	return issuer, redirect, nil
}

func oidcURLSchemeAllowed(u *url.URL, allowLoopbackHTTP bool) bool {
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && allowLoopbackHTTP && oidcLoopbackHost(u.Hostname())
}

func oidcLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func oidcEndpointAllowed(issuer *url.URL, raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.Host != "" && u.Fragment == "" &&
		strings.EqualFold(u.Scheme, issuer.Scheme) && oidcOriginHost(u) == oidcOriginHost(issuer)
}

func oidcOriginHost(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else if u.Scheme == "http" {
			port = "80"
		}
	}
	return strings.ToLower(u.Hostname()) + ":" + port
}

// forget drops finished callbacks not yet turned into a session, so an
// identity unlinked meanwhile cannot finish one. People mid-sign-in start again.
func (r *oidcRuntime) forget() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.results = map[string]oidcResult{}
	r.mu.Unlock()
}

func (r *oidcRuntime) close() {
	if r == nil {
		return
	}
	r.cancel()
	r.client.CloseIdleConnections()
}

func (r *oidcRuntime) begin(ctx context.Context, now time.Time, ip string, userID int64, family string) (string, string, error) {
	if err := r.ensureProvider(ctx); err != nil {
		return "", "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clean(now)
	if len(r.flows) >= oidcMaxFlows {
		return "", "", errOIDCFlow
	}
	n := 0
	for _, tx := range r.flows {
		if tx.ip == ip {
			n++
		}
	}
	if n >= oidcMaxPerIP {
		return "", "", errOIDCFlow
	}
	state, nonce, verifier, browser := token(32), token(32), token(48), token(32)
	challenge := sha256.Sum256([]byte(verifier))
	r.flows[state] = oidcTransaction{cookie: sum(browser), nonce: nonce, pkce: verifier, ip: ip,
		expires: now.Add(oidcFlowLife), userID: userID, family: family}
	authURL := r.oauth.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:])),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"))
	return authURL, browser, nil
}

func (r *oidcRuntime) takeTransaction(now time.Time, states []string, browser string) (oidcTransaction, error) {
	if len(states) != 1 || states[0] == "" || browser == "" {
		return oidcTransaction{}, errOIDCFlow
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clean(now)
	tx, ok := r.flows[states[0]]
	if !ok || subtle.ConstantTimeCompare([]byte(tx.cookie), []byte(sum(browser))) != 1 || !now.Before(tx.expires) {
		return oidcTransaction{}, errOIDCFlow
	}
	delete(r.flows, states[0])
	return tx, nil
}

func (r *oidcRuntime) exchange(ctx context.Context, code string, tx oidcTransaction) (oidcClaims, error) {
	if code == "" {
		return oidcClaims{}, errOIDCProvider
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ctx = oidc.ClientContext(ctx, r.client)
	select {
	case r.provider <- struct{}{}:
		defer func() { <-r.provider }()
	case <-ctx.Done():
		return oidcClaims{}, errOIDCProvider
	}
	tok, err := r.oauth.Exchange(ctx, code, oauth2.VerifierOption(tx.pkce))
	if err != nil {
		// The one cause the owner can fix in Settings; the provider's own text is not logged.
		if re := (*oauth2.RetrieveError)(nil); errors.As(err, &re) && re.ErrorCode == "invalid_client" {
			log.Print("auth: single sign-on: the provider refused HolyArrs's client id or secret; check them in Settings > Sign-in")
		}
		return oidcClaims{}, errOIDCProvider
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return oidcClaims{}, errOIDCProvider
	}
	id, err := r.verifier.Verify(ctx, raw)
	if err != nil || id.Issuer != r.opt.Issuer || len(id.Audience) == 0 ||
		id.Subject == "" || !ssoText(id.Subject, 255) || subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(tx.nonce)) != 1 {
		return oidcClaims{}, errOIDCProvider
	}
	var extra struct {
		AZP               string `json:"azp"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if id.Claims(&extra) != nil ||
		extra.AZP != "" && extra.AZP != r.opt.ClientID ||
		len(id.Audience) > 1 && extra.AZP != r.opt.ClientID {
		return oidcClaims{}, errOIDCProvider
	}
	display := strings.TrimSpace(extra.Name)
	if !ssoText(display, 200) {
		display = strings.TrimSpace(extra.PreferredUsername)
	}
	if !ssoText(display, 200) {
		display = r.userInfoName(ctx, tok, id.Subject)
	}
	if !ssoText(display, 200) {
		display = "oidc-user"
	}
	return oidcClaims{subject: id.Subject, display: display}, nil
}

// userInfoName asks the userinfo endpoint for a name when the ID token has
// none (Authelia 4.39 sends profile claims only there). The answer counts
// only for the same subject; any failure just leaves the name empty.
func (r *oidcRuntime) userInfoName(ctx context.Context, tok *oauth2.Token, subject string) string {
	r.mu.Lock()
	endpoint := r.userinfo
	r.mu.Unlock()
	if endpoint == "" || tok.AccessToken == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var info struct {
		Subject           string `json:"sub"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&info) != nil || info.Subject != subject {
		return ""
	}
	if name := strings.TrimSpace(info.Name); ssoText(name, 200) {
		return name
	}
	return strings.TrimSpace(info.PreferredUsername)
}

func (r *oidcRuntime) saveResult(now time.Time, claims oidcClaims, tx oidcTransaction) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clean(now)
	if len(r.results) >= oidcMaxFlows {
		return "", errOIDCFlow
	}
	n := 0
	for _, result := range r.results {
		if result.ip == tx.ip {
			n++
		}
	}
	if n >= oidcMaxPerIP {
		return "", errOIDCFlow
	}
	browser := token(32)
	r.results[sum(browser)] = oidcResult{oidcClaims: claims, expires: now.Add(oidcFlowLife), ip: tx.ip, userID: tx.userID, family: tx.family}
	return browser, nil
}

func (r *oidcRuntime) takeResult(now time.Time, browser string) (oidcResult, error) {
	if browser == "" {
		return oidcResult{}, errOIDCFlow
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clean(now)
	key := sum(browser)
	result, ok := r.results[key]
	if !ok || !now.Before(result.expires) {
		return oidcResult{}, errOIDCFlow
	}
	delete(r.results, key)
	return result, nil
}

func (r *oidcRuntime) clean(now time.Time) {
	for state, tx := range r.flows {
		if !now.Before(tx.expires) {
			delete(r.flows, state)
		}
	}
	for key, result := range r.results {
		if !now.Before(result.expires) {
			delete(r.results, key)
		}
	}
}

func (a *Auth) oidcRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/oidc/start", func(w http.ResponseWriter, r *http.Request) { a.oidcStart(w, r, false) })
	m.HandleFunc("POST /api/v1/auth/oidc/link", func(w http.ResponseWriter, r *http.Request) { a.oidcStart(w, r, true) })
	m.HandleFunc("GET /api/v1/auth/oidc/callback", a.oidcCallback)
	m.HandleFunc("POST /api/v1/auth/oidc/finish", func(w http.ResponseWriter, r *http.Request) { a.oidcFinish(w, r, false) })
	m.HandleFunc("POST /api/v1/auth/oidc/link-finish", func(w http.ResponseWriter, r *http.Request) { a.oidcFinish(w, r, true) })
}

func (a *Auth) oidcStart(w http.ResponseWriter, r *http.Request, linking bool) {
	oidcPrivate(w)
	o := a.ssoNow().oidc // read once: a settings save may swap it
	if o == nil {
		fail(w, http.StatusNotFound, "OIDC sign-in is not enabled")
		return
	}
	if webserver.URLBaseFrom(r.Context())+"/api/v1/auth/oidc/callback" != o.redirect.Path {
		fail(w, http.StatusServiceUnavailable, "OIDC callback path is not configured for this address")
		return
	}
	var userID int64
	var family string
	if linking {
		who, ok := a.requireSSOLink(w, r)
		if !ok {
			return
		}
		userID, family = who.UserID, a.currentFamily(r)
		if family == "" {
			fail(w, http.StatusForbidden, "sign in locally again before linking")
			return
		}
	}
	if !a.ssoAttempt(w, r) {
		return
	}
	authURL, browser, err := o.begin(r.Context(), a.now(), a.clientKey(r), userID, family)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "OIDC provider is temporarily unavailable")
		return
	}
	a.setOIDCCookie(w, o, browser, int(oidcFlowLife.Seconds()))
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

func (a *Auth) oidcCallback(w http.ResponseWriter, r *http.Request) {
	oidcPrivate(w)
	if !a.ssoContinue(w, r) {
		return
	}
	o := a.ssoNow().oidc // read once: a settings save may swap it
	if o == nil {
		fail(w, http.StatusBadRequest, "OIDC callback was not accepted")
		return
	}
	if !a.oidcCallbackTrusted(r, o) {
		a.setOIDCCookie(w, o, "", -1)
		fail(w, http.StatusBadRequest, "OIDC callback was not accepted")
		return
	}
	ck, _ := r.Cookie(oidcCookieName)
	browser := ""
	if ck != nil {
		browser = ck.Value
	}
	tx, err := o.takeTransaction(a.now(), r.URL.Query()["state"], browser)
	if err != nil {
		a.setOIDCCookie(w, o, "", -1)
		fail(w, http.StatusBadRequest, "OIDC callback was not accepted")
		return
	}
	// From here the browser came back from the provider: failures go back to
	// the sign-in page with a plain message, never a bare error with the code
	// still in the address bar.
	failed := func() {
		a.setOIDCCookie(w, o, "", -1)
		http.Redirect(w, r, webserver.URLBaseFrom(r.Context())+"/login?sso=failed", http.StatusSeeOther)
	}
	query := r.URL.Query()
	codes, providerErrors := query["code"], query["error"]
	if len(codes) != 1 || codes[0] == "" || len(providerErrors) != 0 {
		failed()
		return
	}
	claims, err := o.exchange(r.Context(), codes[0], tx)
	if err != nil {
		failed()
		return
	}
	resultCookie, err := o.saveResult(a.now(), claims, tx)
	if err != nil {
		failed()
		return
	}
	a.setOIDCCookie(w, o, resultCookie, int(oidcFlowLife.Seconds()))
	to := "/login?sso=finish"
	if tx.userID != 0 {
		to = "/account?sso=finish"
	}
	// Under a URL base the pages live below it too.
	http.Redirect(w, r, webserver.URLBaseFrom(r.Context())+to, http.StatusSeeOther)
}

func (a *Auth) oidcFinish(w http.ResponseWriter, r *http.Request, linking bool) {
	oidcPrivate(w)
	a.ssoMu.RLock() // until the session exists: see ssoMu
	defer a.ssoMu.RUnlock()
	o := a.ssoNow().oidc // read once: a settings save may swap it
	if o == nil {
		fail(w, http.StatusNotFound, "OIDC sign-in is not enabled")
		return
	}
	if !a.ssoContinue(w, r) {
		return
	}
	ck, _ := r.Cookie(oidcCookieName)
	browser := ""
	if ck != nil {
		browser = ck.Value
	}
	result, err := o.takeResult(a.now(), browser)
	a.setOIDCCookie(w, o, "", -1)
	if err != nil || linking != (result.userID != 0) {
		fail(w, http.StatusBadRequest, "OIDC sign-in could not be continued")
		return
	}
	if linking {
		who, ok := a.requireSSOLink(w, r)
		if !ok {
			return
		}
		if who.UserID != result.userID || result.family == "" || a.currentFamily(r) != result.family {
			fail(w, http.StatusForbidden, "sign in locally again before linking")
			return
		}
		if _, _, err := a.ssoLinkIdentity(r, "oidc", o.opt.Issuer, result.subject, result.display, result.userID, result.family); err != nil {
			fail(w, http.StatusConflict, "OIDC identity could not be linked")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"linked": true})
		a.ssoPassed(r)
		return
	}
	id, name, err := a.ssoIdentity(r.Context(), "oidc", o.opt.Issuer, result.subject, result.display, o.opt.AllowJoin, 0)
	if err != nil {
		a.failedEvent(r, outsideName(result.display), ssoRefusal("oidc", err))
		fail(w, http.StatusUnauthorized, "OIDC identity is not allowed to sign in")
		return
	}
	a.ssoPassed(r)
	a.passedAs(w, r, id, name, "oidc")
}

func (a *Auth) oidcCallbackTrusted(r *http.Request, o *oidcRuntime) bool {
	scheme := "http"
	if r.TLS != nil || a.behindHTTPS(r) {
		scheme = "https"
	}
	host := &url.URL{Scheme: scheme, Host: r.Host}
	return strings.EqualFold(scheme, o.redirect.Scheme) && oidcOriginHost(host) == oidcOriginHost(o.redirect) &&
		webserver.URLBaseFrom(r.Context())+r.URL.Path == o.redirect.Path
}

func (a *Auth) setOIDCCookie(w http.ResponseWriter, o *oidcRuntime, value string, maxAge int) {
	path := strings.TrimSuffix(o.redirect.Path, "/api/v1/auth/oidc/callback")
	if path == "" {
		path = "/"
	} else {
		path += "/"
	}
	http.SetCookie(w, &http.Cookie{Name: oidcCookieName, Value: value, Path: path, HttpOnly: true,
		Secure: o.redirect.Scheme == "https", SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func oidcPrivate(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
