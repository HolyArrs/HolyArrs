package accounts

// A refused sign-in callback: the log says which check failed, a person in a
// browser goes back to the sign-in page, and nothing secret is written down.

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// captureOIDCLog collects the log until the test ends, with a fresh line budget.
func captureOIDCLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	oidcRefusalLog.Lock()
	oidcRefusalLog.from, oidcRefusalLog.n = time.Time{}, 0
	oidcRefusalLog.Unlock()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func TestOIDCTakeTransactionTellsTheReasonsApart(t *testing.T) {
	f := newFakeOIDC(t)
	r, err := newOIDCRuntime(f.options())
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	now := time.Now()
	authURL, browser, err := r.begin(context.Background(), now, "ip:test", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	state := u.Query().Get("state")
	tests := []struct {
		name    string
		at      time.Time
		states  []string
		browser string
		want    *oidcRefusal
	}{
		{"no state", now, nil, browser, errOIDCNoState},
		{"empty state", now, []string{""}, browser, errOIDCNoState},
		{"two states", now, []string{state, state}, browser, errOIDCNoState},
		{"no cookie", now, []string{state}, "", errOIDCNoCookie},
		{"unknown state", now, []string{"someone-elses-state"}, browser, errOIDCUnknown},
		{"another browser's cookie", now, []string{state}, "another-browser", errOIDCCookie},
		{"too late", now.Add(oidcFlowLife), []string{state}, browser, errOIDCUnknown},
	}
	for _, tc := range tests {
		_, err := r.takeTransaction(tc.at, tc.states, tc.browser)
		if err != tc.want {
			t.Errorf("%s: error = %#v, want %q", tc.name, err, tc.want.why)
		}
		// Whatever the reason, callers and the browser see the one error.
		if !errorsIs(err, errOIDCFlow) || err.Error() != errOIDCFlow.Error() {
			t.Errorf("%s: error text = %q", tc.name, err)
		}
	}
}

func TestOIDCRefusedCallbackLogsTheReasonAndSendsABrowserToSignIn(t *testing.T) {
	provider := newFakeOIDC(t)
	f := gated(t, Options{SSO: SSOOptions{OIDC: pointer(provider.options())}})
	const host = "127.0.0.1:18745"
	const code = "code-must-not-be-logged"
	navigate := map[string]string{"Sec-Fetch-Mode": "navigate"}
	started := client(t)
	authURL, startCookie := startOIDC(t, f, started, "/api/v1/auth/oidc/start", "")
	u, _ := url.Parse(authURL)
	state := u.Query().Get("state")
	callback := f.srv.URL + "/api/v1/auth/oidc/callback?state=" + url.QueryEscape(state) + "&code=" + code

	tests := []struct {
		name    string
		url     string
		host    string
		cookie  string
		headers map[string]string
		page    bool // sent to the sign-in page, not the bare error
		why     string
	}{
		{"another browser, no cookie", callback, host, "", navigate, true, "no sign-in cookie"},
		{"old browser without Sec-Fetch headers", callback, host, "", map[string]string{"Accept": "text/html,application/xhtml+xml"}, true, "no sign-in cookie"},
		{"another browser's cookie", callback, host, "cookie-must-not-be-logged", navigate, true, "cookie does not match"},
		{"unknown state", f.srv.URL + "/api/v1/auth/oidc/callback?state=state-must-not-be-logged&code=" + code, host, startCookie, navigate, true, "unknown or expired state"},
		{"no state", f.srv.URL + "/api/v1/auth/oidc/callback?code=" + code, host, startCookie, navigate, true, "no state, or more than one"},
		{"another address", callback, "wrong.example", startCookie, navigate, true, "host or path differs from the redirect URL"},
		{"a script, no cookie", callback, host, "", nil, false, "no sign-in cookie"},
		{"a page's own fetch", callback, host, "", map[string]string{"Sec-Fetch-Mode": "cors", "Accept": "text/html"}, false, "no sign-in cookie"},
		{"a script at another address", callback, "wrong.example", startCookie, nil, false, "host or path differs from the redirect URL"},
	}
	for _, tc := range tests {
		buf := captureOIDCLog(t)
		req, _ := http.NewRequest(http.MethodGet, tc.url, nil)
		req.Host = tc.host
		for k, v := range tc.headers {
			req.Header.Set(k, v)
		}
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: oidcCookieName, Value: tc.cookie})
		}
		c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(resp.Body)
		resp.Body.Close()
		if tc.page {
			if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login?sso=elsewhere" {
				t.Errorf("%s: %d %q, want the sign-in page", tc.name, resp.StatusCode, resp.Header.Get("Location"))
			}
		} else if resp.StatusCode != http.StatusBadRequest || strings.TrimSpace(body.String()) != `{"error":"OIDC callback was not accepted"}` {
			t.Errorf("%s: %d %q, want the bare 400", tc.name, resp.StatusCode, body)
		}
		// Neither answer says which check failed.
		if strings.Contains(body.String()+resp.Header.Get("Location"), tc.why) {
			t.Errorf("%s: the answer tells the reason: %q", tc.name, body)
		}
		out := buf.String()
		if strings.Count(out, "\n") != 1 || !strings.Contains(out, "auth: single sign-on: callback refused: "+tc.why+"\n") {
			t.Errorf("%s: log = %q, want one line with %q", tc.name, out, tc.why)
		}
		for _, secret := range []string{state, startCookie, code, "state-must-not-be-logged", "cookie-must-not-be-logged", "wrong.example", "callback?", "state="} {
			if strings.Contains(out, secret) {
				t.Errorf("%s: log holds %q: %q", tc.name, secret, out)
			}
		}
	}

	// None of those used up the sign-in: the browser that started it still finishes.
	o := f.a.ssoNow().oidc
	o.mu.Lock()
	nonce := o.flows[state].nonce
	o.mu.Unlock()
	provider.mu.Lock()
	provider.nonce = nonce
	provider.mu.Unlock()
	buf := captureOIDCLog(t)
	resp, body := oidcCall(t, started, http.MethodGet, callback, host, "", navigate)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login?sso=finish" {
		t.Fatalf("the starting browser: %d %q %s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	if strings.Contains(buf.String(), "callback refused") {
		t.Fatalf("an accepted callback logged a refusal: %q", buf)
	}
}

func TestOIDCCallbackWithoutSingleSignOnStaysABareError(t *testing.T) {
	f := gated(t, Options{})
	buf := captureOIDCLog(t)
	resp, body := oidcCall(t, client(t), http.MethodGet, f.srv.URL+"/api/v1/auth/oidc/callback?state=s&code=c", "127.0.0.1:18745", "", map[string]string{"Sec-Fetch-Mode": "navigate"})
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "OIDC callback was not accepted") {
		t.Fatalf("no single sign-on: %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(buf.String(), "callback refused: single sign-on is not set up") {
		t.Fatalf("log = %q", buf)
	}
}

func TestOIDCRefusalLogStopsAFlood(t *testing.T) {
	buf := captureOIDCLog(t)
	now := time.Now()
	for i := 0; i < oidcRefusalLogPerMinute+15; i++ {
		logOIDCRefusal(now.Add(time.Duration(i)*time.Second), "no sign-in cookie")
	}
	if n := strings.Count(buf.String(), "\n"); n != oidcRefusalLogPerMinute {
		t.Fatalf("%d lines in one minute, want %d", n, oidcRefusalLogPerMinute)
	}
	logOIDCRefusal(now.Add(time.Minute), "no sign-in cookie")
	if n := strings.Count(buf.String(), "\n"); n != oidcRefusalLogPerMinute+1 {
		t.Fatalf("a new minute logged %d lines in all", n)
	}
}
