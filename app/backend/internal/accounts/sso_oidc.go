package accounts

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
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
	GroupsClaim       string            // claim that carries groups; "" turns roles from groups off
	GroupRoles        map[string]string // group to "admin" or "member"
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
	fresh   bool // asked the provider for a real new sign-in
}

type oidcClaims struct {
	subject  string
	display  string
	authTime time.Time // the provider's auth_time; zero when it sent none
	role     string    // role from groups; "" leaves the role alone
}

type oidcResult struct {
	oidcClaims
	expires time.Time
	ip      string
	userID  int64
	family  string
	freshAt time.Time // set only when a fresh sign-in was asked for and checked
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
		RedirectURL: r.opt.RedirectURL, Scopes: oidcScopes(r.opt)}
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

// oidcSiblingHosts lists providers whose discovery document puts the token,
// key and user-info endpoints on other hosts the provider owns. Google signs
// in on accounts.google.com but hands out tokens and keys from googleapis.com.
var oidcSiblingHosts = map[string][]string{
	"accounts.google.com:443": {"oauth2.googleapis.com:443", "www.googleapis.com:443", "openidconnect.googleapis.com:443"},
}

func oidcEndpointAllowed(issuer *url.URL, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" || u.Fragment != "" || !strings.EqualFold(u.Scheme, issuer.Scheme) {
		return false
	}
	host, own := oidcOriginHost(u), oidcOriginHost(issuer)
	if host == own {
		return true
	}
	return strings.EqualFold(issuer.Scheme, "https") && slices.Contains(oidcSiblingHosts[own], host)
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

func (r *oidcRuntime) begin(ctx context.Context, now time.Time, ip string, userID int64, family string, fresh bool) (string, string, error) {
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
		expires: now.Add(oidcFlowLife), userID: userID, family: family, fresh: fresh}
	opts := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:])),
		oauth2.SetAuthURLParam("code_challenge_method", "S256")}
	if fresh {
		// The provider must ask again, not pass a signed-in browser straight through.
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "login"), oauth2.SetAuthURLParam("max_age", "0"))
	}
	authURL := r.oauth.AuthCodeURL(state, opts...)
	return authURL, browser, nil
}

func (r *oidcRuntime) takeTransaction(now time.Time, states []string, browser string) (oidcTransaction, error) {
	if len(states) != 1 || states[0] == "" {
		return oidcTransaction{}, errOIDCNoState
	}
	if browser == "" {
		return oidcTransaction{}, errOIDCNoCookie
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clean(now)
	tx, ok := r.flows[states[0]]
	if !ok || !now.Before(tx.expires) {
		return oidcTransaction{}, errOIDCUnknown
	}
	if subtle.ConstantTimeCompare([]byte(tx.cookie), []byte(sum(browser))) != 1 {
		return oidcTransaction{}, errOIDCCookie
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
		AuthTime          int64  `json:"auth_time"`
	}
	if id.Claims(&extra) != nil ||
		extra.AZP != "" && extra.AZP != r.opt.ClientID ||
		len(id.Audience) > 1 && extra.AZP != r.opt.ClientID {
		return oidcClaims{}, errOIDCProvider
	}
	var all map[string]any
	_ = id.Claims(&all)
	role := groupRole(r.opt.GroupsClaim, r.opt.GroupRoles, all)
	display := strings.TrimSpace(extra.Name)
	if !ssoText(display, 200) {
		display = strings.TrimSpace(extra.PreferredUsername)
	}
	var info map[string]any
	if !ssoText(display, 200) || role == "" && r.opt.GroupsClaim != "" {
		info = r.userInfo(ctx, tok, id.Subject)
	}
	if !ssoText(display, 200) {
		display = userInfoName(info)
	}
	if role == "" {
		role = groupRole(r.opt.GroupsClaim, r.opt.GroupRoles, info)
	}
	if !ssoText(display, 200) {
		display = "oidc-user"
	}
	var at time.Time
	if extra.AuthTime > 0 {
		at = time.Unix(extra.AuthTime, 0)
	}
	return oidcClaims{subject: id.Subject, display: display, authTime: at, role: role}, nil
}

// userInfo asks the userinfo endpoint for what the ID token lacks: a name or
// groups (Authelia 4.39 sends profile and group claims only there). The
// answer counts only for the same subject; any failure just returns nil.
func (r *oidcRuntime) userInfo(ctx context.Context, tok *oauth2.Token, subject string) map[string]any {
	r.mu.Lock()
	endpoint := r.userinfo
	r.mu.Unlock()
	if endpoint == "" || tok.AccessToken == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var info map[string]any
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info) != nil {
		return nil
	}
	if sub, _ := info["sub"].(string); sub != subject {
		return nil
	}
	return info
}

// userInfoName is the name from a userinfo answer, or "".
func userInfoName(info map[string]any) string {
	name, _ := info["name"].(string)
	if name = strings.TrimSpace(name); ssoText(name, 200) {
		return name
	}
	user, _ := info["preferred_username"].(string)
	return strings.TrimSpace(user)
}

// oidcScopes asks for groups too when roles come from groups; most
// providers send the groups claim only for this scope.
func oidcScopes(o OIDCOptions) []string {
	if o.GroupsClaim != "" && len(o.GroupRoles) > 0 {
		return []string{oidc.ScopeOpenID, "profile", "groups"}
	}
	return []string{oidc.ScopeOpenID, "profile"}
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
	res := oidcResult{oidcClaims: claims, expires: now.Add(oidcFlowLife), ip: tx.ip, userID: tx.userID, family: tx.family}
	if tx.fresh && oidcFreshEnough(claims.authTime, now) {
		res.freshAt = claims.authTime
	}
	r.results[sum(browser)] = res
	return browser, nil
}

// oidcFreshEnough says whether the provider's sign-in time is recent enough to
// stand in for a password: inside the recent window, and not more than a
// minute ahead of this clock.
func oidcFreshEnough(at, now time.Time) bool {
	return !at.IsZero() && !at.After(now.Add(time.Minute)) && now.Sub(at) < localRecentFor
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
	// {"fresh": true} asks the provider for a real new sign-in, the proof a
	// person with no HolyArrs password gives before a protected change.
	var body struct {
		Fresh bool `json:"fresh"`
	}
	if !linking {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body)
	}
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
	if !a.oidcOnRedirectOrigin(r, o) {
		writeJSON(w, http.StatusOK, oidcElsewhere(o, linking, body.Fresh))
		return
	}
	if !a.ssoAttempt(w, r) {
		return
	}
	authURL, browser, err := o.begin(r.Context(), a.now(), a.clientKey(r), userID, family, body.Fresh)
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
		logOIDCRefusal(a.now(), "single sign-on is not set up")
		fail(w, http.StatusBadRequest, "OIDC callback was not accepted")
		return
	}
	// The browser that came back is not the one that started (a home-screen
	// app, an in-app browser, another address), or it took too long: a person
	// lands on the sign-in page with a plain message. Scripts and API clients
	// keep the bare 400, and neither answer says which check failed.
	refused := func(why string) {
		logOIDCRefusal(a.now(), why)
		a.setOIDCCookie(w, o, "", -1)
		if oidcBrowserNavigation(r) {
			http.Redirect(w, r, webserver.URLBaseFrom(r.Context())+"/login?sso=elsewhere", http.StatusSeeOther)
			return
		}
		fail(w, http.StatusBadRequest, "OIDC callback was not accepted")
	}
	if !a.oidcCallbackTrusted(r, o) {
		refused("host or path differs from the redirect URL")
		return
	}
	ck, _ := r.Cookie(oidcCookieName)
	browser := ""
	if ck != nil {
		browser = ck.Value
	}
	tx, err := o.takeTransaction(a.now(), r.URL.Query()["state"], browser)
	if err != nil {
		why := "sign-in could not be continued"
		if re := (*oidcRefusal)(nil); errors.As(err, &re) {
			why = re.why
		}
		refused(why)
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
	id, name, err := a.ssoIdentityRole(r.Context(), "oidc", o.opt.Issuer, result.subject, result.display, o.opt.AllowJoin, result.role)
	if err != nil {
		a.failedEvent(r, outsideName(result.display), ssoRefusal("oidc", err))
		fail(w, http.StatusUnauthorized, "OIDC identity is not allowed to sign in")
		return
	}
	a.ssoPassed(r)
	if !result.freshAt.IsZero() {
		r = r.WithContext(withOutsideProof(r.Context(), result.freshAt))
	}
	a.passedAs(w, r, id, name, "oidc")
}

func (a *Auth) oidcCallbackTrusted(r *http.Request, o *oidcRuntime) bool {
	return a.oidcOnRedirectOrigin(r, o) && webserver.URLBaseFrom(r.Context())+r.URL.Path == o.redirect.Path
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
