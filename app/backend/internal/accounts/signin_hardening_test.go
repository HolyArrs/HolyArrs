package accounts

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func proxyAuth(t *testing.T, cidr string) *Auth {
	t.Helper()
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	return &Auth{proxies: []*net.IPNet{n}}
}

func reqFrom(remote string, hdr [][2]string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	for _, h := range hdr {
		r.Header.Add(h[0], h[1])
	}
	return r
}

// The address a trusted proxy forwards is read so that a header the visitor
// wrote can never pick the address: not for the local-network skip, not for
// the sign-in limit key.
func TestForwardedClientCannotBeSpoofed(t *testing.T) {
	a := proxyAuth(t, "172.18.0.0/16")
	for _, c := range []struct {
		name string
		hdr  [][2]string
		seen string // "" means the local skip must not apply
		key  string
	}{
		// nginx with only "X-Real-IP $remote_addr": the visitor's own
		// X-Forwarded-For comes through untouched.
		{"nginx real-ip only, spoofed xff", [][2]string{{"X-Real-IP", "203.0.113.9"}, {"X-Forwarded-For", "192.168.1.5"}}, "", "xri:203.0.113.9"},
		{"nginx real-ip only", [][2]string{{"X-Real-IP", "203.0.113.9"}}, "203.0.113.9", "ip:203.0.113.9"},
		// nginx with both lines: the visitor's entry is on the left.
		{"nginx both lines, spoofed xff", [][2]string{{"X-Real-IP", "203.0.113.9"}, {"X-Forwarded-For", "192.168.1.5, 203.0.113.9"}}, "203.0.113.9", "ip:203.0.113.9"},
		// HAProxy "option forwardfor" adds its own second line.
		{"haproxy two lines", [][2]string{{"X-Forwarded-For", "192.168.1.5"}, {"X-Forwarded-For", "203.0.113.9"}}, "203.0.113.9", "ip:203.0.113.9"},
		// A proxy that passes the visitor's X-Real-IP through: the headers differ,
		// so the limit key is X-Real-IP (the one nginx sets) and the skip is off.
		{"spoofed real-ip, proxy xff", [][2]string{{"X-Real-IP", "192.168.1.5"}, {"X-Forwarded-For", "203.0.113.9"}}, "", "xri:192.168.1.5"},
		// Trusted hops on the right are skipped.
		{"chain of trusted proxies", [][2]string{{"X-Forwarded-For", "192.168.1.5, 203.0.113.9, 172.18.0.7"}}, "203.0.113.9", "ip:203.0.113.9"},
		{"local visitor", [][2]string{{"X-Forwarded-For", "192.168.1.5"}, {"X-Real-IP", "192.168.1.5"}}, "192.168.1.5", "ip:192.168.1.5"},
		{"garbage xff", [][2]string{{"X-Forwarded-For", "not an address"}}, "", "ip:172.18.0.1"},
	} {
		r := reqFrom("172.18.0.1:4000", c.hdr)
		seen, bad := a.seenAddress(r)
		if c.seen == "" && !bad || c.seen != "" && (bad || seen != c.seen) {
			t.Errorf("%s: seen %q bad %v, want %q", c.name, seen, bad, c.seen)
		}
		if got := a.ClientKey(r); got != c.key {
			t.Errorf("%s: key %q, want %q", c.name, got, c.key)
		}
	}
}

// With no trusted proxy, forwarding headers are ignored as before.
func TestForwardedHeadersIgnoredWithoutTrustedProxy(t *testing.T) {
	a := &Auth{}
	r := reqFrom("198.51.100.4:4000", [][2]string{{"X-Forwarded-For", "192.168.1.5"}, {"X-Real-IP", "192.168.1.5"}})
	if seen, bad := a.seenAddress(r); !bad || seen != "" {
		t.Errorf("seen %q bad %v, want refused", seen, bad)
	}
	if got := a.ClientKey(r); got != "ip:198.51.100.4" {
		t.Errorf("key %q", got)
	}
	if a.localPeer(r) {
		t.Error("local skip from an untrusted peer")
	}
}

// localPeer, the skip itself, follows the same rule.
func TestLocalPeerUsesSharedRule(t *testing.T) {
	a := proxyAuth(t, "127.0.0.1/32")
	l, err := parseLocalNets([]string{"192.168.1.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	a.localNets.Store(l)
	for _, c := range []struct {
		name string
		hdr  [][2]string
		want bool
	}{
		{"spoofed xff with real-ip", [][2]string{{"X-Real-IP", "203.0.113.9"}, {"X-Forwarded-For", "192.168.1.5"}}, false},
		{"haproxy two lines", [][2]string{{"X-Forwarded-For", "192.168.1.5"}, {"X-Forwarded-For", "203.0.113.9"}}, false},
		{"real local visitor", [][2]string{{"X-Forwarded-For", "192.168.1.5"}}, true},
	} {
		if got := a.localPeer(reqFrom("127.0.0.1:4000", c.hdr)); got != c.want {
			t.Errorf("%s: local %v, want %v", c.name, got, c.want)
		}
	}
}

// Strangers cannot keep the owner out of a place they signed in from
// before; anywhere else the name's wait holds, the right password included.
func TestNameBackOffDoesNotLockOwnerOut(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, f, client(t))
	right := `{"name":"luis","password":"` + pw + `"}`
	wrong := `{"name":"luis","password":"wrong password"}`
	login := func(from, body string) int {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", body, map[string]string{"X-Forwarded-For": from})
		return resp.StatusCode
	}
	if got := login("203.0.113.50", right); got != http.StatusOK {
		t.Fatalf("first sign-in: %d", got)
	}
	for i := 0; i < freeUserTries+2; i++ {
		login("198.51.100."+strconv.Itoa(i+1), wrong)
	}
	if got := login("198.51.100.1", right); got != http.StatusTooManyRequests {
		t.Fatalf("right password from an address that never signed in: %d, want 429", got)
	}
	if got := login("203.0.113.50", right); got != http.StatusOK {
		t.Fatalf("owner from a place used before: %d, want 200", got)
	}
}

// The per-address limit still holds for the right password.
func TestAddressBackOffStillHolds(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, f, client(t))
	h := map[string]string{"X-Forwarded-For": "198.51.100.9"}
	for i := 0; i < freeTries+1; i++ {
		call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"wrong password"}`, h)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, h); resp.StatusCode != 429 {
		t.Fatalf("right password during the address wait: %d", resp.StatusCode)
	}
}

func TestNameProblemInvisibleCharacters(t *testing.T) {
	for _, n := range []string{"lu​is", "luis‍", "‮luis", "lu\tis", "lu is", "lu　is", "lu\x00is"} {
		if nameProblem(n) == "" {
			t.Errorf("%q accepted", n)
		}
	}
	for _, n := range []string{"luis", "Luis Pérez", "名前", "a-b_c.d", "anna 2"} {
		if msg := nameProblem(n); msg != "" {
			t.Errorf("%q refused: %s", n, msg)
		}
	}
}

func TestSetupRefusesInvisibleName(t *testing.T) {
	f := setup(t, Options{})
	resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"lu​is","password":"`+pw+`"}`, nil)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "hidden") {
		t.Fatalf("setup with a hidden character: %d %s", resp.StatusCode, body)
	}
}

func TestInviteRefusesInvisibleName(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"days":1}`)
	resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"lu\u200bis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "hidden") {
		t.Fatalf("look-alike owner name: %d %s", resp.StatusCode, body)
	}
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"ana","password":"`+pw+`"}`, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("plain name after a refused one: %d", resp.StatusCode)
	}
}
