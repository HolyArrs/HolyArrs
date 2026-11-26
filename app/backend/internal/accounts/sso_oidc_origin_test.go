package accounts

// A sign-in started at another address of the server than the redirect URL's:
// no flow and no cookie there, the browser is told where to go on.

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func oidcFlowCount(f *fixture) int {
	o := f.a.ssoNow().oidc
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.flows)
}

func TestOIDCStartAtTheRedirectAddressBeginsAFlow(t *testing.T) {
	provider := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	authURL, cookie := startOIDC(t, f, client(t), "/api/v1/auth/oidc/start", "")
	if !strings.HasPrefix(authURL, provider.srv.URL+"/auth?") || cookie == "" || oidcFlowCount(f) != 1 {
		t.Fatalf("start at the redirect address: %q, cookie %t, %d flows", authURL, cookie != "", oidcFlowCount(f))
	}
}

func TestOIDCStartAtAnotherAddressContinuesAtTheRedirectAddress(t *testing.T) {
	provider := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	// The fixture's own address is another port than the redirect URL's.
	// Nothing the request says about itself ends up in the answer.
	for i := 0; i < freeTries+2; i++ {
		resp, body := call(t, client(t), http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/start", "",
			map[string]string{"X-Forwarded-Host": "evil.example", "X-Forwarded-Proto": "https"})
		if resp.StatusCode != http.StatusOK || body != "{\"continue_at\":\"http://127.0.0.1:18745/login?sso=start\"}\n" {
			t.Fatalf("start at another address: %d %s", resp.StatusCode, body)
		}
		if len(resp.Cookies()) != 0 {
			t.Fatalf("start at another address set cookies: %v", resp.Cookies())
		}
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
		}
	}
	if n := oidcFlowCount(f); n != 0 {
		t.Fatalf("start at another address began %d flows", n)
	}
	// None of those counted toward the start limit.
	resp, body := oidcCall(t, client(t), http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/start", "127.0.0.1:18745", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "auth_url") {
		t.Fatalf("start at the redirect address after those: %d %s", resp.StatusCode, body)
	}
}

func TestOIDCLinkAtAnotherAddressNamesTheRedirectAddress(t *testing.T) {
	provider := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	c := client(t)
	csrf := setupOwner(t, f, c)
	resp, body := call(t, c, http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/link", "", map[string]string{CSRFHeader: csrf})
	if resp.StatusCode != http.StatusOK || body != "{\"link_at\":\"http://127.0.0.1:18745\"}\n" {
		t.Fatalf("link at another address: %d %s", resp.StatusCode, body)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == oidcCookieName {
			t.Fatal("link at another address set the flow cookie")
		}
	}
	if n := oidcFlowCount(f); n != 0 {
		t.Fatalf("link at another address began %d flows", n)
	}
	// Still only for someone signed in.
	resp, body = call(t, client(t), http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/link", "", nil)
	if resp.StatusCode == http.StatusOK || strings.Contains(body, "link_at") {
		t.Fatalf("link without a session: %d %s", resp.StatusCode, body)
	}
}

// A proof asked for at another address ({"fresh": true}) stays one where the
// browser goes on, and the start there still asks the provider again.
func TestOIDCFreshStartAtAnotherAddressStaysFresh(t *testing.T) {
	provider := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	resp, body := call(t, client(t), http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/start", `{"fresh":true}`, nil)
	var at struct {
		ContinueAt string `json:"continue_at"`
		AuthURL    string `json:"auth_url"`
	}
	jsonOf(t, body, &at)
	if resp.StatusCode != http.StatusOK || at.AuthURL != "" || at.ContinueAt != "http://127.0.0.1:18745/login?sso=start&fresh=1" {
		t.Fatalf("fresh start at another address: %d %s", resp.StatusCode, body)
	}
	if n := oidcFlowCount(f); n != 0 || len(resp.Cookies()) != 0 {
		t.Fatalf("fresh start at another address: %d flows, cookies %v", n, resp.Cookies())
	}
	resp, body = oidcCall(t, client(t), http.MethodPost, f.srv.URL+"/api/v1/auth/oidc/start", "127.0.0.1:18745", `{"fresh":true}`, nil)
	jsonOf(t, body, &at)
	u, _ := url.Parse(at.AuthURL)
	if resp.StatusCode != http.StatusOK || u.Query().Get("prompt") != "login" || u.Query().Get("max_age") != "0" {
		t.Fatalf("fresh start at the redirect address: %d %s", resp.StatusCode, body)
	}
}
