package accounts

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Signing in an AI app: instead of pasting an AI key, an AI
// app such as Claude Code or Cursor opens HolyArrs in the browser, the person
// signs in as usual and allows it, and the app gets an AI key of its own
// (scope mcp) that shows and can be revoked under Settings > AI connections.
// This is the MCP authorization flow: OAuth 2.1 with PKCE (S256), public
// clients only, with open client registration (RFC 7591) or a client ID
// metadata document: an https client_id whose JSON names the app (ai_client_docs.go).

const (
	aiCodeFor     = 5 * time.Minute
	aiCodesMax    = 1000
	aiCodesEach   = 10
	aiClientIDMax = 4 << 10
	aiRedirectMax = 10
)

// aiConnectPage is the web page where a person allows an AI app.
const aiConnectPage = "/ai"

const aiClientPrefix = "hai_"

// aiClient is a registered AI app. Registration is open to anyone, so the
// client id simply carries the registration (name and return addresses):
// nothing is stored, and the person sees both before allowing.
type aiClient struct {
	Name      string   `json:"n"`
	Redirects []string `json:"r"`
	// ID is the client_id the app sent (not part of the registration).
	ID string `json:"-"`
}

// aiCode is an allowed sign-in waiting for the app to fetch its key.
type aiCode struct {
	client    aiClient
	redirect  string
	challenge string
	resource  string
	user      int64
	until     time.Time
}

type aiCodes struct {
	mu sync.Mutex
	m  map[string]aiCode
}

// aiRequest is a checked sign-in request from an AI app.
type aiRequest struct {
	client    aiClient
	redirect  string
	challenge string
	state     string
	resource  string
}

func (c aiClient) id() string {
	b, _ := json.Marshal(c)
	return aiClientPrefix + base64.RawURLEncoding.EncodeToString(b)
}

func parseAIClient(id string) (aiClient, bool) {
	raw, ok := strings.CutPrefix(id, aiClientPrefix)
	if !ok || len(id) > aiClientIDMax {
		return aiClient{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return aiClient{}, false
	}
	var c aiClient
	if json.Unmarshal(b, &c) != nil || len(c.Redirects) == 0 || len(c.Redirects) > aiRedirectMax {
		return aiClient{}, false
	}
	for _, r := range c.Redirects {
		if !aiRedirectAllowed(r) {
			return aiClient{}, false
		}
	}
	c.Name = aiClientName(c.Name)
	c.ID = id
	return c, true
}

// aiClientFor reads a client_id: a registration made here, or an https
// address with the app's metadata document.
func (a *Auth) aiClientFor(ctx context.Context, id string) (aiClient, bool) {
	if strings.HasPrefix(id, "https://") {
		return a.cimd.client(ctx, id)
	}
	return parseAIClient(id)
}

// aiClientName is the name shown to the person and used as the key label.
func aiClientName(n string) string {
	// Control and bidi characters could make one name look like another.
	n = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0x200E || r == 0x200F || r == 0x061C {
			return ' '
		}
		return r
	}, n)
	n = strings.Join(strings.Fields(n), " ")
	if n == "" {
		return "AI app"
	}
	if r := []rune(n); len(r) > 48 {
		n = string(r[:48])
	}
	return n
}

// aiRedirectAllowed takes https addresses, http on this computer (CLI
// apps listen on a local port) and an app's own scheme (cursor://).
func aiRedirectAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil || len(raw) > 512 {
		return false
	}
	switch s := strings.ToLower(u.Scheme); s {
	case "https":
		return u.Host != ""
	case "http":
		return loopbackHost(u.Hostname())
	case "", "javascript", "data", "file", "vbscript", "about", "blob", "ftp", "ws", "wss",
		// Schemes that hand an address on to a browser or a system tool.
		"microsoft-edge", "x-safari-https", "x-safari-http", "googlechrome", "googlechromes", "firefox",
		"intent", "search-ms", "ms-msdt", "ms-officecmd", "ms-search":
		return false
	default:
		// An app's own scheme is cursor://..., never scheme:https://...
		// (a way to open a website through it).
		if u.Opaque != "" || strings.Contains(strings.ToLower(u.Host+u.Path), "://") {
			return false
		}
		for _, r := range s {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '.' || r == '-') {
				return false
			}
		}
		return true
	}
}

func loopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// redirectMatches compares a return address with a registered one. On this
// computer the port may differ (RFC 8252 7.3): CLI apps pick a free one.
func redirectMatches(registered, got string) bool {
	if registered == got {
		return true
	}
	a, err1 := url.Parse(registered)
	b, err2 := url.Parse(got)
	if err1 != nil || err2 != nil || !strings.EqualFold(a.Scheme, "http") || !strings.EqualFold(b.Scheme, "http") {
		return false
	}
	return loopbackHost(a.Hostname()) && strings.EqualFold(a.Hostname(), b.Hostname()) && a.Path == b.Path && a.RawQuery == b.RawQuery
}

func (c *aiCodes) put(now time.Time, code string, v aiCode) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]aiCode{}
	}
	for k, x := range c.m {
		if now.After(x.until) {
			delete(c.m, k)
		}
	}
	mine := 0
	for _, x := range c.m {
		if x.user == v.user {
			mine++
		}
	}
	if len(c.m) >= aiCodesMax || mine >= aiCodesEach {
		return false
	}
	c.m[sum(code)] = v
	return true
}

func (c *aiCodes) take(now time.Time, code string) (aiCode, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[sum(code)]
	delete(c.m, sum(code))
	return v, ok && !now.After(v.until)
}

// aiOrigin is this HolyArrs as the app reached it, base path included.
func (a *Auth) aiOrigin(r *http.Request) string {
	scheme := "http"
	if a.behindHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host + webserver.URLBaseFrom(r.Context())
}

// aiResourceMetadataURL is where an AI app learns how to sign in (sent in
// WWW-Authenticate on the AI connection route), for the address it used:
// the app checks that the two match.
func (a *Auth) aiResourceMetadataURL(r *http.Request) string {
	return a.aiOrigin(r) + "/.well-known/oauth-protected-resource" + aiRoutePath(r.URL.Path)
}

// aiRoutePath is the AI route an address names: /mcp or /api/v1/mcp.
func aiRoutePath(p string) string {
	if strings.TrimSuffix(p, "/") == "/mcp" || p == "mcp" {
		return "/mcp"
	}
	return "/api/v1/mcp"
}

func (a *Auth) aiOAuthRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /.well-known/oauth-protected-resource", a.aiResource)
	m.HandleFunc("GET /.well-known/oauth-protected-resource/{path...}", a.aiResource)
	m.HandleFunc("GET /.well-known/oauth-authorization-server", a.aiServer)
	m.HandleFunc("POST /oauth/register", a.aiRegister)
	m.HandleFunc("GET /oauth/authorize", a.aiAuthorize)
	m.HandleFunc("POST /oauth/token", a.aiToken)
	m.HandleFunc("GET /api/v1/auth/ai-connect", a.aiConnectInfo)
	m.HandleFunc("POST /api/v1/auth/ai-connect", a.aiConnectAllow)
}

// aiPublic are the sign-in routes an AI app calls before it has a key.
func aiPublic(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "GET /.well-known/oauth-authorization-server", "POST /oauth/register", "GET /oauth/authorize", "POST /oauth/token":
		return true
	}
	return r.Method == http.MethodGet && (r.URL.Path == "/.well-known/oauth-protected-resource" ||
		strings.HasPrefix(r.URL.Path, "/.well-known/oauth-protected-resource/"))
}

// aiCORS lets pages read the sign-in answers (none use cookies). Apps that
// register from a browser page are not served: there is no preflight.
func aiCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

func (a *Auth) aiResource(w http.ResponseWriter, r *http.Request) {
	aiCORS(w)
	o := a.aiOrigin(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 o + aiRoutePath(r.PathValue("path")),
		"authorization_servers":    []string{o},
		"scopes_supported":         []string{webserver.ScopeMCP},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "HolyArrs",
	})
}

func (a *Auth) aiServer(w http.ResponseWriter, r *http.Request) {
	aiCORS(w)
	o := a.aiOrigin(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         o,
		"authorization_endpoint":                         o + "/oauth/authorize",
		"token_endpoint":                                 o + "/oauth/token",
		"registration_endpoint":                          o + "/oauth/register",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"scopes_supported":                               []string{webserver.ScopeMCP},
		"authorization_response_iss_parameter_supported": true,
		"client_id_metadata_document_supported":          true,
	})
}

func oauthFail(w http.ResponseWriter, code int, kind, msg string) {
	aiCORS(w)
	writeJSON(w, code, map[string]string{"error": kind, "error_description": msg})
}

func (a *Auth) aiRegister(w http.ResponseWriter, r *http.Request) {
	// Each address may register a few apps, then waits like a failed sign-in.
	key := "aireg:" + a.clientKey(r)
	a.mu.Lock()
	wait := a.waitFor(key, freeTries)
	if wait == 0 {
		a.noteFail(key)
	}
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		oauthFail(w, http.StatusTooManyRequests, "temporarily_unavailable", "Too many new apps from this address. Try again in a few minutes.")
		return
	}
	var in struct {
		Name       string   `json:"client_name"`
		Redirects  []string `json:"redirect_uris"`
		AuthMethod string   `json:"token_endpoint_auth_method"`
		Grants     []string `json:"grant_types"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		oauthFail(w, http.StatusBadRequest, "invalid_client_metadata", "send JSON client metadata")
		return
	}
	if len(in.Redirects) == 0 || len(in.Redirects) > aiRedirectMax {
		oauthFail(w, http.StatusBadRequest, "invalid_redirect_uri", "send 1 to 10 redirect_uris")
		return
	}
	for _, u := range in.Redirects {
		if !aiRedirectAllowed(u) {
			oauthFail(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris must be https, http on localhost, or an app's own scheme")
			return
		}
	}
	c := aiClient{Name: aiClientName(in.Name), Redirects: in.Redirects}
	id := c.id()
	if len(id) > aiClientIDMax {
		oauthFail(w, http.StatusBadRequest, "invalid_client_metadata", "client metadata too long")
		return
	}
	aiCORS(w)
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  id,
		"client_id_issued_at":        a.now().Unix(),
		"client_name":                c.Name,
		"redirect_uris":              c.Redirects,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// checkAIRequest reads an authorize request: bad is a bad client or return
// address, kind any other fault.
func (a *Auth) checkAIRequest(ctx context.Context, q url.Values) (req aiRequest, bad bool, errKind, msg string) {
	c, ok := a.aiClientFor(ctx, q.Get("client_id"))
	if !ok {
		return req, true, "invalid_client", "This AI app is not registered with HolyArrs. Remove it in the AI app and add it again."
	}
	redirect := q.Get("redirect_uri")
	if redirect == "" && len(c.Redirects) == 1 {
		redirect = c.Redirects[0]
	}
	if !slices.ContainsFunc(c.Redirects, func(reg string) bool { return redirectMatches(reg, redirect) }) {
		return req, true, "invalid_request", "The AI app sent a return address it did not register."
	}
	req = aiRequest{client: c, redirect: redirect, challenge: q.Get("code_challenge"), state: q.Get("state"), resource: q.Get("resource")}
	switch {
	case q.Get("response_type") != "code":
		return req, false, "unsupported_response_type", "only response_type=code"
	case q.Get("code_challenge_method") != "S256" || len(req.challenge) < 43 || len(req.challenge) > 128:
		return req, false, "invalid_request", "PKCE with S256 is required"
	// Some apps (Gemini) send a long encrypted state; 4 KB still bounds it.
	case len(req.state) > 4<<10 || len(req.resource) > 1024:
		return req, false, "invalid_request", "state or resource too long"
	}
	if s := q.Get("scope"); s != "" && !slices.Contains(strings.Fields(s), webserver.ScopeMCP) {
		return req, false, "invalid_scope", "the only scope is mcp"
	}
	return req, false, "", ""
}

// backTo is the app's return address with the answer added.
func backTo(redirect string, add url.Values) string {
	u, _ := url.Parse(redirect)
	q := u.Query()
	for k, v := range add {
		if v[0] != "" {
			q.Set(k, v[0])
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func aiErrorPage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>HolyArrs</title>` +
		`<body style="font:16px system-ui;background:#111;color:#eee;padding:24px"><h1 style="font-size:20px">Could not connect the AI app</h1><p>` +
		html.EscapeString(msg) + `</p></body>`))
}

// aiAuthorize checks the request and hands it to the web page where the
// person signs in (if needed) and allows the app.
func (a *Auth) aiAuthorize(w http.ResponseWriter, r *http.Request) {
	// Faults are shown here, never sent back: anyone can register any return
	// address, so sending to it before the person allows would make this an
	// open redirect.
	if _, bad, kind, msg := a.checkAIRequest(r.Context(), r.URL.Query()); bad || kind != "" {
		aiErrorPage(w, msg)
		return
	}
	base := webserver.URLBaseFrom(r.Context())
	page := aiConnectPage + "?" + r.URL.RawQuery
	if _, ok := a.session(w, r); !ok {
		http.Redirect(w, r, base+"/login?next="+url.QueryEscape(page), http.StatusFound)
		return
	}
	http.Redirect(w, r, base+page, http.StatusFound)
}

// aiConnectInfo tells the allow page what is asking.
func (a *Auth) aiConnectInfo(w http.ResponseWriter, r *http.Request) {
	req, bad, kind, msg := a.checkAIRequest(r.Context(), r.URL.Query())
	if bad || kind != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	u, _ := url.Parse(req.redirect)
	back, local := u.Host, false
	if u.Scheme != "http" && u.Scheme != "https" {
		// An app's own scheme: show the whole address.
		back = truncate(req.redirect, 80)
	} else if loopbackHost(u.Hostname()) {
		back, local = "this computer ("+u.Host+")", true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"client_name": req.client.Name,
		"returns_to":  back,
		"local":       local,
		// The name is whatever the app sent; only a return address on this computer is trusted.
		"name_verified": local,
		"enabled":       a.aiOn(r.Context()),
		"deny_url":      backTo(req.redirect, url.Values{"error": {"access_denied"}, "state": {req.state}, "iss": {a.aiOrigin(r)}}),
	})
}

// aiConnectAllow is the person allowing the app: it gets a one-time code.
func (a *Auth) aiConnectAllow(w http.ResponseWriter, r *http.Request) {
	if !a.aiOn(r.Context()) {
		fail(w, http.StatusForbidden, "AI connections are off")
		return
	}
	if !webserver.RequireStrong(w, r) {
		return
	}
	who, _ := webserver.From(r.Context())
	if who.Via != "session" {
		fail(w, http.StatusForbidden, "allow AI apps from your browser")
		return
	}
	var in struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, `send {"query": "<the sign-in request>"}`)
		return
	}
	q, err := url.ParseQuery(in.Query)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad sign-in request")
		return
	}
	req, bad, kind, msg := a.checkAIRequest(r.Context(), q)
	if bad || kind != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	code := token(32)
	if !a.codes.put(a.now(), code, aiCode{client: req.client, redirect: req.redirect, challenge: req.challenge, resource: req.resource,
		user: who.UserID, until: a.now().Add(aiCodeFor)}) {
		fail(w, http.StatusTooManyRequests, "too many AI apps are signing in; try again in a few minutes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"redirect": backTo(req.redirect, url.Values{"code": {code}, "state": {req.state}, "iss": {a.aiOrigin(r)}})})
}

// aiToken swaps a code for the app's own AI key.
func (a *Auth) aiToken(w http.ResponseWriter, r *http.Request) {
	if !a.aiOn(r.Context()) {
		oauthFail(w, http.StatusForbidden, "access_denied", "AI connections are off")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		oauthFail(w, http.StatusBadRequest, "invalid_request", "send a form")
		return
	}
	f := r.PostForm
	if f.Get("grant_type") != "authorization_code" {
		oauthFail(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code")
		return
	}
	c, ok := a.codes.take(a.now(), f.Get("code"))
	if !ok {
		oauthFail(w, http.StatusBadRequest, "invalid_grant", "the code is wrong, used or expired; sign in again")
		return
	}
	verifier := sha256.Sum256([]byte(f.Get("code_verifier")))
	if f.Get("client_id") != c.client.ID || !redirectMatches(c.redirect, f.Get("redirect_uri")) && f.Get("redirect_uri") != "" ||
		subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(verifier[:])), []byte(c.challenge)) != 1 ||
		f.Get("resource") != "" && c.resource != "" && f.Get("resource") != c.resource {
		oauthFail(w, http.StatusBadRequest, "invalid_grant", "the code does not match this app or its PKCE verifier")
		return
	}
	k, err := a.issueAIKey(r, c)
	if err != nil {
		log.Printf("auth: AI sign-in key: %v", err)
		oauthFail(w, http.StatusInternalServerError, "server_error", "something went wrong; see the log")
		return
	}
	aiCORS(w)
	writeJSON(w, http.StatusOK, map[string]any{"access_token": k, "token_type": "Bearer", "scope": webserver.ScopeMCP})
}

// issueAIKey makes the signed-in app's AI key, owned by the person who
// allowed it. It lasts until revoked, like an AI key made by hand.
func (a *Auth) issueAIKey(r *http.Request, c aiCode) (string, error) {
	state, err := stateOfUser(r.Context(), a.s.Read(), c.user)
	if err != nil || state != "active" {
		if err == nil {
			err = sql.ErrNoRows
		}
		return "", err
	}
	k := keyPrefix + token(32)
	now := database.FormatTime(a.now())
	by := domain.Actor{Kind: "user", ID: c.user}
	actor, _ := json.Marshal(by)
	scopes, _ := json.Marshal([]string{webserver.ScopeMCP})
	var id int64
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		var err error
		id, err = insertKey(r.Context(), tx, now, string(actor), c.user, c.client.Name, string(scopes), sum(k), k[len(k)-4:])
		return err
	})
	if err != nil {
		return "", err
	}
	a.event(r.WithContext(context.WithoutCancel(r.Context())), by, "api_key_created", "AI app "+c.client.Name+" signed in",
		map[string]any{"key_id": id, "label": c.client.Name, "scopes": []string{webserver.ScopeMCP}, "via": "sign_in"})
	return k, nil
}

// aiOn says AI connections are on (Settings > AI connections; the API
// package owns the switch, this_app mcp_enabled).
func (a *Auth) aiOn(ctx context.Context) bool {
	return database.SettingOr(ctx, a.s.Read(), "this_app", "mcp_enabled", "") == "on"
}
