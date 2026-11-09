package accounts

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// turnAIOn flips the AI apps switch the ai package owns.
func turnAIOn(t *testing.T, f *fixture) {
	t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		return database.PutSetting(tx, "this_app", "mcp_enabled", "on", domain.System, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAIAllowRefusedWhenOff(t *testing.T) {
	f := setup(t, Options{})
	m := http.NewServeMux()
	f.a.Routes(m)
	f.srv.Config.Handler = f.a.Middleware(m)
	c := client(t)
	csrf := setupOwner(t, f, c)
	bare := &http.Client{}
	_, body := call(t, bare, "POST", f.srv.URL+"/oauth/register", `{"client_name":"x","redirect_uris":["http://localhost:4567/callback"]}`, nil)
	var reg struct {
		ClientID string `json:"client_id"`
	}
	_ = json.Unmarshal([]byte(body), &reg)
	h := sha256.Sum256([]byte("a-verifier-that-is-long-enough-for-pkce-0123456789"))
	q := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {"http://localhost:4567/callback"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(h[:])}, "code_challenge_method": {"S256"}, "state": {"s"}}
	allowBody, _ := json.Marshal(map[string]string{"query": q.Encode()})
	if resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/ai-connect", string(allowBody), map[string]string{CSRFHeader: csrf}); resp.StatusCode != 403 {
		t.Fatalf("allow while off: %d %s", resp.StatusCode, body)
	}
	req, _ := http.NewRequest("POST", f.srv.URL+"/oauth/token", strings.NewReader("grant_type=authorization_code&code=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if resp, err := bare.Do(req); err != nil || resp.StatusCode != 403 {
		t.Fatalf("token while off: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}
	var n int
	if err := f.s.Read().QueryRow("SELECT count(*) FROM api_keys").Scan(&n); err != nil || n != 0 {
		t.Fatalf("keys: %d %v", n, err)
	}
}

func TestAIClientNameStripsBidi(t *testing.T) {
	if got := aiClientName("Good‮evil"); strings.ContainsAny(got, "‮") || got != "Good evil" {
		t.Fatalf("got %q", got)
	}
}

func TestAIRegisterLimited(t *testing.T) {
	f := setup(t, Options{})
	m := http.NewServeMux()
	f.a.Routes(m)
	f.srv.Config.Handler = f.a.Middleware(m)
	bare := &http.Client{}
	for i := 0; i < 30; i++ {
		resp, body := call(t, bare, "POST", f.srv.URL+"/oauth/register", `{"client_name":"x","redirect_uris":["http://localhost:4567/callback"]}`, nil)
		if resp.StatusCode == 429 {
			if i < freeTries {
				t.Fatalf("limited after %d", i)
			}
			if !strings.Contains(body, "Too many new apps from this address. Try again in a few minutes.") {
				t.Fatalf("limit message: %s", body)
			}
			return
		}
	}
	t.Fatal("never limited")
}

// signInAIApp runs the sign-in an AI app does (MCP authorization): learn
// where to sign in, register, open the browser, allow, swap the code.
func TestAIAppSignsInWithoutAPastedKey(t *testing.T) {
	f := setup(t, Options{})
	turnAIOn(t, f)
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/titles", okHandler)
	f.a.Routes(m)
	f.srv.Config.Handler = f.a.Middleware(mcpEcho(webserver.RecordRouteHits(m)))
	c := client(t)
	csrf := setupOwner(t, f, c)
	bare := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// 1. No key: 401 says where to read how to sign in.
	resp, _ := call(t, bare, "POST", f.srv.URL+"/api/v1/mcp", `{}`, nil)
	wa := resp.Header.Get("WWW-Authenticate")
	if resp.StatusCode != 401 || !strings.Contains(wa, `resource_metadata="`+f.srv.URL+`/.well-known/oauth-protected-resource/api/v1/mcp"`) {
		t.Fatalf("no key: %d %q", resp.StatusCode, wa)
	}
	// Even beside a browser session the AI route asks for its own sign-in.
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/mcp", `{}`, map[string]string{CSRFHeader: csrf}); resp.StatusCode != 401 {
		t.Fatalf("session on the AI route: %d", resp.StatusCode)
	}

	// 2. Metadata.
	_, body := call(t, bare, "GET", f.srv.URL+"/.well-known/oauth-protected-resource/api/v1/mcp", "", nil)
	var prm struct {
		Resource string   `json:"resource"`
		AS       []string `json:"authorization_servers"`
	}
	_ = json.Unmarshal([]byte(body), &prm)
	if prm.Resource != f.srv.URL+"/api/v1/mcp" || len(prm.AS) != 1 || prm.AS[0] != f.srv.URL {
		t.Fatalf("resource metadata: %s", body)
	}
	_, body = call(t, bare, "GET", f.srv.URL+"/.well-known/oauth-authorization-server", "", nil)
	var as map[string]any
	_ = json.Unmarshal([]byte(body), &as)
	if as["registration_endpoint"] != f.srv.URL+"/oauth/register" || as["token_endpoint"] != f.srv.URL+"/oauth/token" {
		t.Fatalf("server metadata: %s", body)
	}

	// 3. Register: an unsafe return address is refused.
	if resp, _ := call(t, bare, "POST", f.srv.URL+"/oauth/register", `{"client_name":"x","redirect_uris":["http://evil.example/cb"]}`, nil); resp.StatusCode != 400 {
		t.Fatalf("http return address on another host: %d", resp.StatusCode)
	}
	if resp, _ := call(t, bare, "POST", f.srv.URL+"/oauth/register", `{"client_name":"x","redirect_uris":["javascript:alert(1)"]}`, nil); resp.StatusCode != 400 {
		t.Fatalf("javascript return address: %d", resp.StatusCode)
	}
	resp, body = call(t, bare, "POST", f.srv.URL+"/oauth/register", `{"client_name":"Claude Code","redirect_uris":["http://localhost:4567/callback"]}`, nil)
	var reg struct {
		ClientID string `json:"client_id"`
	}
	_ = json.Unmarshal([]byte(body), &reg)
	if resp.StatusCode != 201 || reg.ClientID == "" {
		t.Fatalf("register: %d %s", resp.StatusCode, body)
	}

	// 4. Authorize: the app may use another local port; signed out goes to
	// the sign-in page first, signed in to the allow page.
	verifier := "a-verifier-that-is-long-enough-for-pkce-0123456789"
	h := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {"http://localhost:5678/callback"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(h[:])}, "code_challenge_method": {"S256"}, "state": {"st8"},
		"resource": {f.srv.URL + "/api/v1/mcp"}, "scope": {"mcp"}}
	resp, _ = call(t, bare, "GET", f.srv.URL+"/oauth/authorize?"+q.Encode(), "", nil)
	if loc := resp.Header.Get("Location"); resp.StatusCode != 302 || !strings.HasPrefix(loc, "/login?next=%2Fai%3F") {
		t.Fatalf("signed out: %d %q", resp.StatusCode, loc)
	}
	resp, _ = call(t, c, "GET", f.srv.URL+"/oauth/authorize?"+q.Encode(), "", nil)
	if loc := resp.Header.Get("Location"); resp.StatusCode != 302 || loc != "/ai?"+q.Encode() {
		t.Fatalf("signed in: %d %q", resp.StatusCode, loc)
	}
	// A return address it never registered is shown, not followed.
	bad := url.Values{}
	for k, v := range q {
		bad[k] = v
	}
	bad.Set("redirect_uri", "https://evil.example/cb")
	if resp, _ := call(t, c, "GET", f.srv.URL+"/oauth/authorize?"+bad.Encode(), "", nil); resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
		t.Fatalf("unregistered return address: %d", resp.StatusCode)
	}
	// A long encrypted state (Gemini sends one over 1 KB) is fine.
	long := url.Values{}
	for k, v := range q {
		long[k] = v
	}
	long.Set("state", strings.Repeat("s", 2000))
	if resp, _ := call(t, c, "GET", f.srv.URL+"/oauth/authorize?"+long.Encode(), "", nil); resp.StatusCode != 302 {
		t.Fatalf("long state: %d", resp.StatusCode)
	}
	// No PKCE is shown here too: the return address is not followed before
	// the person allows (no open redirect).
	nopkce := url.Values{"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {"http://localhost:4567/callback"}, "state": {"s"}}
	resp, _ = call(t, c, "GET", f.srv.URL+"/oauth/authorize?"+nopkce.Encode(), "", nil)
	if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
		t.Fatalf("no PKCE: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// 5. The allow page: what is asking, then allow.
	_, body = call(t, c, "GET", f.srv.URL+"/api/v1/auth/ai-connect?"+q.Encode(), "", nil)
	if !strings.Contains(body, `"client_name":"Claude Code"`) || !strings.Contains(body, "this computer") {
		t.Fatalf("info: %s", body)
	}
	allowBody, _ := json.Marshal(map[string]string{"query": q.Encode()})
	if resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/ai-connect", string(allowBody), nil); resp.StatusCode != 403 {
		t.Fatalf("allow without CSRF: %d", resp.StatusCode)
	}
	_, body = call(t, c, "POST", f.srv.URL+"/api/v1/auth/ai-connect", string(allowBody), map[string]string{CSRFHeader: csrf})
	var allow struct {
		Redirect string `json:"redirect"`
	}
	_ = json.Unmarshal([]byte(body), &allow)
	back, err := url.Parse(allow.Redirect)
	if err != nil || back.Host != "localhost:5678" || back.Query().Get("state") != "st8" || back.Query().Get("code") == "" {
		t.Fatalf("allow: %s", body)
	}
	codeVal := back.Query().Get("code")

	// 6. Token: a wrong verifier burns the code.
	form := func(code, v string) string {
		return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {reg.ClientID},
			"redirect_uri": {"http://localhost:5678/callback"}, "code_verifier": {v}, "resource": {f.srv.URL + "/api/v1/mcp"}}.Encode()
	}
	tok := func(body string) (*http.Response, string) {
		req, _ := http.NewRequest("POST", f.srv.URL+"/oauth/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := bare.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	if resp, _ := tok(form(codeVal, "wrong-verifier-wrong-verifier-wrong-verifier-00")); resp.StatusCode != 400 {
		t.Fatalf("wrong verifier: %d", resp.StatusCode)
	}
	if resp, _ := tok(form(codeVal, verifier)); resp.StatusCode != 400 {
		t.Fatalf("code used twice: %d", resp.StatusCode)
	}

	// A fresh allow and the right verifier give an AI key.
	_, body = call(t, c, "POST", f.srv.URL+"/api/v1/auth/ai-connect", string(allowBody), map[string]string{CSRFHeader: csrf})
	_ = json.Unmarshal([]byte(body), &allow)
	back, _ = url.Parse(allow.Redirect)
	resp, body = tok(form(back.Query().Get("code"), verifier))
	var out struct {
		Access string `json:"access_token"`
		Type   string `json:"token_type"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if resp.StatusCode != 200 || out.Type != "Bearer" || !strings.HasPrefix(out.Access, keyPrefix) {
		t.Fatalf("token: %d %s", resp.StatusCode, body)
	}
	var label, scope string
	_ = f.s.Read().QueryRow("SELECT label, scope FROM api_keys ORDER BY id DESC LIMIT 1").Scan(&label, &scope)
	if label != "Claude Code" || scope != `["mcp"]` {
		t.Fatalf("stored key: %q %q", label, scope)
	}
	if resp, body := call(t, bare, "POST", f.srv.URL+"/api/v1/mcp", `{}`, map[string]string{"Authorization": "Bearer " + out.Access}); resp.StatusCode != 200 || body != "AI app Claude Code" {
		t.Fatalf("signed-in key on the AI route: %d %s", resp.StatusCode, body)
	}
	// It reaches nothing else.
	if resp, _ := call(t, bare, "GET", f.srv.URL+"/api/v1/titles", "", map[string]string{"Authorization": "Bearer " + out.Access}); resp.StatusCode != 401 {
		t.Fatalf("bearer off the AI route: %d", resp.StatusCode)
	}
	// A revoked or wrong key is told to sign in again.
	resp, _ = call(t, bare, "POST", f.srv.URL+"/api/v1/mcp", `{}`, map[string]string{"Authorization": "Bearer " + out.Access + "x"})
	if resp.StatusCode != 401 || !strings.Contains(resp.Header.Get("WWW-Authenticate"), `error="invalid_token"`) {
		t.Fatalf("wrong key: %d %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}

func TestAICodeExpires(t *testing.T) {
	var c aiCodes
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	c.put(now, "abc", aiCode{until: now.Add(aiCodeFor)})
	if _, ok := c.take(now.Add(aiCodeFor+time.Second), "abc"); ok {
		t.Fatal("expired code taken")
	}
	// One person cannot fill the waiting sign-ins for everyone.
	for i := range aiCodesEach {
		if !c.put(now, "u1-"+itoa(int64(i)), aiCode{user: 1, until: now.Add(aiCodeFor)}) {
			t.Fatalf("code %d refused", i)
		}
	}
	if c.put(now, "u1-more", aiCode{user: 1, until: now.Add(aiCodeFor)}) || !c.put(now, "u2", aiCode{user: 2, until: now.Add(aiCodeFor)}) {
		t.Fatal("per-person cap")
	}
}

// The short address /mcp gets sign-in details that name /mcp: apps check
// that the two match.
func TestAIResourceFollowsTheAddressUsed(t *testing.T) {
	f := setup(t, Options{})
	m := http.NewServeMux()
	f.a.Routes(m)
	f.srv.Config.Handler = f.a.Middleware(mcpEcho(webserver.RecordRouteHits(m)))
	bare := &http.Client{}
	resp, _ := call(t, bare, "POST", f.srv.URL+"/mcp", `{}`, nil)
	if wa := resp.Header.Get("WWW-Authenticate"); !strings.Contains(wa, `/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("short address: %q", wa)
	}
	for path, want := range map[string]string{"/mcp": "/mcp", "/api/v1/mcp": "/api/v1/mcp", "": "/api/v1/mcp"} {
		u := f.srv.URL + "/.well-known/oauth-protected-resource" + path
		resp, body := call(t, bare, "GET", u, "", nil)
		if resp.StatusCode != 200 || !strings.Contains(body, `"resource":"`+f.srv.URL+want+`"`) {
			t.Fatalf("%s: %d %s", u, resp.StatusCode, body)
		}
	}
	// No OpenID answer: HolyArrs does not offer OpenID sign-in.
	if _, body := call(t, bare, "GET", f.srv.URL+"/.well-known/openid-configuration", "", nil); strings.Contains(body, `"issuer"`) {
		t.Fatal("openid-configuration answered")
	}
}

func TestAIRedirectRules(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://localhost:33418/callback":  true,
		"http://127.0.0.1:9/cb":            true,
		"http://[::1]:9/cb":                true,
		"https://claude.ai/api/mcp/cb":     true,
		"cursor://anysphere.cursor-mcp/cb": true,
		"http://192.168.1.10/cb":           false,
		"javascript:alert(1)":              false,
		"data:text/html,x":                 false,
		"https://a/cb#frag":                false,
		// Schemes that would open a website through another program.
		"microsoft-edge:https://evil.example/cb": false,
		"x-safari-https://evil.example/cb":       false,
		"myapp:https://evil.example/cb":          false,
		"myapp://cb/https://evil.example":        false,
	} {
		if aiRedirectAllowed(raw) != want {
			t.Errorf("%s: want %v", raw, want)
		}
	}
	if !redirectMatches("http://127.0.0.1:1/cb", "http://127.0.0.1:2/cb") || redirectMatches("https://a/cb", "https://a/cb2") ||
		redirectMatches("http://127.0.0.1:1/cb", "http://localhost:2/cb") {
		t.Fatal("redirect matching")
	}
	// A tampered client id with a bad address is refused like an unknown one.
	if _, ok := parseAIClient(aiClient{Name: "x", Redirects: []string{"http://evil.example/cb"}}.id()); ok {
		t.Fatal("bad address in client id")
	}
}

// mcpEcho stands in for the AI route: it answers with the key's label.
func mcpEcho(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mcpPath(r.URL.Path) {
			who, _ := webserver.From(r.Context())
			_, _ = w.Write([]byte("AI app " + who.KeyLabel))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// An app may name itself with an https address (client ID metadata
// document), as the Claude app does by default: no registration needed.
func TestAIAppSignsInWithAMetadataDocument(t *testing.T) {
	f := setup(t, Options{})
	turnAIOn(t, f)
	m := http.NewServeMux()
	f.a.Routes(m)
	f.srv.Config.Handler = f.a.Middleware(mcpEcho(webserver.RecordRouteHits(m)))
	c := client(t)
	csrf := setupOwner(t, f, c)
	var docURL string
	docs := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client.json":
			_, _ = w.Write([]byte(`{"client_id":"` + docURL + `","client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"],"token_endpoint_auth_method":"none"}`))
		case "/other.json":
			_, _ = w.Write([]byte(`{"client_id":"https://elsewhere.example/x","client_name":"x","redirect_uris":["https://claude.ai/cb"]}`))
		default:
			_, _ = w.Write([]byte(strings.Repeat("x", cimdSize+10)))
		}
	}))
	defer docs.Close()
	docURL = docs.URL + "/client.json"
	f.a.cimd.http = docs.Client()

	_, body := call(t, &http.Client{}, "GET", f.srv.URL+"/.well-known/oauth-authorization-server", "", nil)
	if !strings.Contains(body, `"client_id_metadata_document_supported":true`) {
		t.Fatalf("metadata: %s", body)
	}
	h := sha256.Sum256([]byte("a-verifier-that-is-long-enough-for-pkce-0123456789"))
	q := url.Values{"response_type": {"code"}, "client_id": {docURL}, "redirect_uri": {"https://claude.ai/api/mcp/auth_callback"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(h[:])}, "code_challenge_method": {"S256"}, "state": {"s"}}
	_, body = call(t, c, "GET", f.srv.URL+"/api/v1/auth/ai-connect?"+q.Encode(), "", nil)
	if !strings.Contains(body, `"client_name":"Claude"`) || !strings.Contains(body, `"returns_to":"claude.ai"`) {
		t.Fatalf("info: %s", body)
	}
	allow, _ := json.Marshal(map[string]string{"query": q.Encode()})
	_, body = call(t, c, "POST", f.srv.URL+"/api/v1/auth/ai-connect", string(allow), map[string]string{CSRFHeader: csrf})
	var out struct {
		Redirect string `json:"redirect"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	back, _ := url.Parse(out.Redirect)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {back.Query().Get("code")}, "client_id": {docURL},
		"redirect_uri": {"https://claude.ai/api/mcp/auth_callback"}, "code_verifier": {"a-verifier-that-is-long-enough-for-pkce-0123456789"}}
	resp, err := http.PostForm(f.srv.URL+"/oauth/token", form)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var got map[string]string
	_ = json.Unmarshal(b, &got)
	if resp.StatusCode != 200 || !strings.HasPrefix(got["access_token"], keyPrefix) {
		t.Fatalf("token: %d", resp.StatusCode)
	}
	// A document naming another client_id, or one too big, is refused.
	for _, bad := range []string{docs.URL + "/other.json", docs.URL + "/big.json"} {
		q.Set("client_id", bad)
		if resp, _ := call(t, c, "GET", f.srv.URL+"/oauth/authorize?"+q.Encode(), "", nil); resp.StatusCode != 400 {
			t.Fatalf("%s: %d", bad, resp.StatusCode)
		}
	}
}

func TestAIMetadataDocumentsOnlyFromPublicAddresses(t *testing.T) {
	var c aiCIMD
	for _, id := range []string{"https://127.0.0.1/client.json", "https://192.168.1.75/c.json", "http://example.com/c.json", "https://example.com/"} {
		if _, ok := c.client(context.Background(), id); ok {
			t.Errorf("%s accepted", id)
		}
	}
}
