package accounts

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Strangers spreading wrong guesses over many addresses still hit the
// name's wait: a fresh address gets no free guess once the name is held.
func TestFreshAddressesCannotSprayName(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, f, client(t))
	wrong := `{"name":"luis","password":"wrong password"}`
	login := func(from, body string) int {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", body, map[string]string{"X-Forwarded-For": from})
		return resp.StatusCode
	}
	for i := 0; i < freeUserTries; i++ {
		login("198.51.100."+strconv.Itoa(i+1), wrong)
	}
	for i := 0; i < 5; i++ {
		if got := login("192.0.2."+strconv.Itoa(i+1), wrong); got != http.StatusTooManyRequests {
			t.Fatalf("wrong guess from fresh address %d during the name wait: %d, want 429", i, got)
		}
	}
}

// The owner signing in from a place they used before is not held by the
// name's wait, even after a typo there.
func TestOwnerKnownPlaceSkipsNameWait(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, f, client(t))
	right := `{"name":"luis","password":"` + pw + `"}`
	wrong := `{"name":"luis","password":"wrong password"}`
	login := func(from, body string) int {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", body, map[string]string{"X-Forwarded-For": from})
		return resp.StatusCode
	}
	if got := login("203.0.113.50", right); got != http.StatusOK {
		t.Fatalf("first sign-in from home: %d", got)
	}
	for i := 0; i < freeUserTries+2; i++ {
		login("198.51.100."+strconv.Itoa(i+1), wrong)
	}
	if got := login("203.0.113.50", wrong); got != http.StatusUnauthorized {
		t.Fatalf("typo from home: %d, want 401", got)
	}
	if got := login("203.0.113.50", right); got != http.StatusOK {
		t.Fatalf("owner from home after a typo during the name wait: %d, want 200", got)
	}
	// A known place lasts days, not minutes.
	f.clock.add(3 * 24 * time.Hour)
	for i := 0; i < freeUserTries+2; i++ {
		login("198.51.101."+strconv.Itoa(i+1), wrong)
	}
	if got := login("203.0.113.50", right); got != http.StatusOK {
		t.Fatalf("owner from home days later: %d, want 200", got)
	}
}

// The two-factor step of a password sign-in uses the same rule: strangers
// holding the name's wait cannot stop the owner between password and code.
func TestTwoFactorStepNotHeldByNameWait(t *testing.T) {
	f := gated(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	own := ownerOf(t, f)
	secret, _ := turnOnTwoFactor(t, f, own)
	home := map[string]string{"X-Forwarded-For": "203.0.113.50"}
	c := client(t)
	f.clock.add(totpStep * time.Second)
	// Home becomes a known place only once the whole sign-in, code included, is done.
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, home)
	var first struct {
		Ticket string `json:"ticket"`
	}
	_ = json.Unmarshal([]byte(out), &first)
	if resp, out = call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+first.Ticket+`","code":"`+codeNow(t, f, secret, 0)+`"}`, home); resp.StatusCode != 200 {
		t.Fatalf("first sign-in: %d %s", resp.StatusCode, out)
	}
	f.clock.add(totpStep * time.Second)
	resp, out = call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, home)
	var v struct {
		Ticket string `json:"ticket"`
	}
	_ = json.Unmarshal([]byte(out), &v)
	if resp.StatusCode != 200 || v.Ticket == "" {
		t.Fatalf("password step: %d %s", resp.StatusCode, out)
	}
	f.clock.add(totpStep * time.Second) // a fresh code; the clock stays put from here
	for i := 0; i < freeUserTries+2; i++ {
		call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"wrong password"}`,
			map[string]string{"X-Forwarded-For": "198.51.100." + strconv.Itoa(i+1)})
	}
	resp, out = call(t, c, "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+v.Ticket+`","code":"`+codeNow(t, f, secret, 0)+`"}`, home)
	if resp.StatusCode != 200 {
		t.Fatalf("right code during the name wait: %d %s", resp.StatusCode, out)
	}
}

// Cloudflare (or another proxy that adds X-Forwarded-For) in front of an
// nginx that sets only X-Real-IP: the headers differ on every request. Each
// visitor keeps their own limit key (the X-Real-IP the trusted proxy set) instead
// of sharing the proxy's.
func TestDisagreeingHeadersKeepPerClientKey(t *testing.T) {
	a := proxyAuth(t, "172.18.0.0/16")
	r := reqFrom("172.18.0.1:4000", [][2]string{{"X-Forwarded-For", "198.51.100.7"}, {"X-Real-IP", "104.16.0.1"}})
	if got := a.ClientKey(r); got != "xri:104.16.0.1" {
		t.Errorf("key %q, want the X-Real-IP client", got)
	}
	if _, bad := a.seenAddress(r); !bad {
		t.Error("local skip allowed on disagreeing headers")
	}
}

func TestNameJoinersInsideWords(t *testing.T) {
	for _, n := range []string{"علی\u200cرضا", "क्\u200dष", "👩\u200d💻"} {
		if msg := nameProblem(n); msg != "" {
			t.Errorf("%q refused: %s", n, msg)
		}
	}
	for _, n := range []string{"lu\u200dis", "lu\u200cis", "\u200cعلی", "علی\u200c", "علی\u200c\u200cرضا"} {
		if nameProblem(n) == "" {
			t.Errorf("%q accepted", n)
		}
	}
}

func TestResetFileByteOrderMark(t *testing.T) {
	for _, in := range []string{"\ufeffluis\n" + pw + "\n", "\ufeffname: luis\npassword: " + pw + "\n"} {
		name, password, err := ReadNewOwner(strings.NewReader(in))
		if err != nil || name != "luis" || password != pw {
			t.Errorf("%q: %q %q %v", in, name, password, err)
		}
	}
}
