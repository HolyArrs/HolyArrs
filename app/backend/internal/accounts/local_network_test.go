package accounts

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Only the listed networks count as local; an empty list counts nobody.
func TestIsLocalOnlyListedNetworks(t *testing.T) {
	a := &Auth{}
	if a.isLocal("127.0.0.1:1") {
		t.Fatal("no list, yet loopback counts as local")
	}
	l, err := parseLocalNets([]string{" 192.168.1.0/24 ", "fd00::/8", "", "10.0.0.7"})
	if err != nil {
		t.Fatal(err)
	}
	a.localNets.Store(l)
	for addr, want := range map[string]bool{
		"192.168.1.5:1": true, "[::ffff:192.168.1.9]:1": true, "[fd00::1]:1": true, "10.0.0.7:1": true, "10.0.0.7": true,
		"10.0.0.8:1": false, "192.168.2.1:1": false, "127.0.0.1:1": false, "[::1]:1": false, "8.8.8.8:1": false, "nonsense": false,
	} {
		if got := a.isLocal(addr); got != want {
			t.Errorf("isLocal(%s) = %v", addr, got)
		}
	}
}

func TestParseLocalNets(t *testing.T) {
	l, err := parseLocalNets([]string{" 192.168.1.20/24", "10.0.0.1", "10.0.0.1/32", "[fd00::1]", "\t"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(l.list, ","); got != "192.168.1.0/24,10.0.0.1/32,fd00::1/128" {
		t.Fatalf("list %s", got)
	}
	for _, bad := range []string{"home", "192.168.1.0/33", "0.0.0.0/0", "::/0", "300.1.1.1"} {
		if _, err := parseLocalNets([]string{"10.0.0.0/8", bad}); err == nil || !strings.Contains(err.Error(), bad) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	many := make([]string, maxLocalNets+1)
	for i := range many {
		many[i] = fmt.Sprintf("10.0.%d.0/24", i)
	}
	if _, err := parseLocalNets(many); err == nil {
		t.Fatal("33 networks taken")
	}
	if _, err := parseLocalNets(many[:maxLocalNets]); err != nil {
		t.Fatal(err)
	}
}

// Turning the skip on needs a list, and then only the listed networks skip.
func TestSkipOnlyOnListedNetworks(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	h := map[string]string{CSRFHeader: csrf}
	put := func(body string, want int) string {
		t.Helper()
		resp, out := call(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", body, h)
		if resp.StatusCode != want {
			t.Fatalf("PUT %s: %d %s", body, resp.StatusCode, out)
		}
		return out
	}
	anon := func() int {
		resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", nil)
		return resp.StatusCode
	}
	if out := put(`{"skip_login_local_network":true}`, 400); !strings.Contains(out, "local network first") {
		t.Fatalf("empty list refusal: %s", out)
	}
	if _, out := call(t, c, "GET", f.srv.URL+"/api/v1/auth/settings", "", nil); !strings.Contains(out, `"skip_login_local_network":false`) || !strings.Contains(out, `"local_networks":[]`) {
		t.Fatalf("settings after refusal: %s", out)
	}
	if out := put(`{"local_networks":["192.168.1.0/24","home"]}`, 400); !strings.Contains(out, `home`) {
		t.Fatalf("bad entry not named: %s", out)
	}
	out := put(`{"skip_login_local_network":true,"local_networks":[" 192.168.1.0/24 ",""]}`, 200)
	if !strings.Contains(out, `"local_networks":["192.168.1.0/24"]`) {
		t.Fatalf("saved list: %s", out)
	}
	// The test client is on loopback, which is not listed.
	if got := anon(); got != 401 {
		t.Fatalf("unlisted loopback skipped sign-in: %d", got)
	}
	put(`{"local_networks":["192.168.1.0/24","127.0.0.1"]}`, 200)
	if got := anon(); got != 200 {
		t.Fatalf("listed loopback: %d", got)
	}
	if out := put(`{"local_networks":[]}`, 400); !strings.Contains(out, "local network first") {
		t.Fatalf("emptying the list while skipping: %s", out)
	}
	put(`{"skip_login_local_network":false}`, 200)
	put(`{"local_networks":[]}`, 200)
}

// Behind a trusted proxy, the forwarded address is the one checked.
func TestSkipListUsesForwardedAddress(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1"}})
	c := client(t)
	csrf := setupOwner(t, f, c)
	call(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["192.168.1.0/24"]}`, map[string]string{CSRFHeader: csrf})
	for ip, want := range map[string]int{"192.168.1.7": 200, "10.0.0.9": 401, "8.8.8.8": 401} {
		if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", map[string]string{"X-Forwarded-For": ip}); resp.StatusCode != want {
			t.Errorf("forwarded %s: %d, want %d", ip, resp.StatusCode, want)
		}
	}
}

// A saved list is read back when HolyArrs starts.
func TestLocalNetsLoadedAtStart(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	call(t, c, "PUT", f.srv.URL+"/api/v1/auth/settings", `{"local_networks":["192.168.1.0/24","fd00::/8"]}`, map[string]string{CSRFHeader: csrf})
	a := New(f.s, Options{fastHash: true})
	t.Cleanup(a.Close)
	if l := a.localNets.Load(); l == nil || strings.Join(l.list, ",") != "192.168.1.0/24,fd00::/8" {
		t.Fatalf("loaded %v", l)
	}
}

// The skip is only for direct home-network visits, or ones a listed proxy vouches for.
func TestSkipLocalStrict(t *testing.T) {
	nets, err := parseLocalNets([]string{"10.0.0.0/8", "192.168.1.0/24", "172.16.0.0/12"})
	if err != nil {
		t.Fatal(err)
	}
	try := func(a *Auth, peer, host string, xff string) bool {
		r := httptest.NewRequest("GET", "/api/v1/titles", nil)
		r.RemoteAddr, r.Host = peer+":5000", host
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return a.strictLocal(r)
	}
	plain := setup(t, Options{}).a
	plain.hosts["app.example.com"] = true
	plain.localNets.Store(nets)
	plain.skipLocal.Store(true)
	if try(plain, "10.0.0.5", "app.example.com", "") {
		t.Error("public host name skipped sign-in")
	}
	if !try(plain, "10.0.0.5", "10.0.0.9:6262", "") {
		t.Error("direct visit by IP did not skip")
	}
	if try(plain, "172.17.0.1", "10.0.0.9:6262", "192.168.1.20") {
		t.Error("forwarding headers from an unlisted proxy skipped sign-in")
	}
	proxied := setup(t, Options{TrustedProxies: []string{"172.17.0.1"}}).a
	proxied.localNets.Store(nets)
	proxied.skipLocal.Store(true)
	if !try(proxied, "172.17.0.1", "10.0.0.9:6262", "192.168.1.20") {
		t.Error("listed proxy with a home client did not skip")
	}
	if try(proxied, "172.17.0.1", "10.0.0.9:6262", "8.8.8.8") {
		t.Error("listed proxy with an outside client skipped sign-in")
	}
}

// A reverse proxy that drops its forwarding headers and sets Host to the
// upstream IP (nginx's default) must not pass internet visitors off as home
// ones: its own address never earns the skip, whether it is listed as a
// trusted proxy or was seen forwarding before. Plain home visits still skip.
func TestSkipLocalRefusesProxyPeer(t *testing.T) {
	nets, err := parseLocalNets([]string{"192.168.1.0/24", "172.16.0.0/12"})
	if err != nil {
		t.Fatal(err)
	}
	try := func(a *Auth, peer, xff string) bool {
		r := httptest.NewRequest("GET", "/api/v1/titles", nil)
		r.RemoteAddr, r.Host = peer+":5000", "192.168.1.9:6262"
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return a.strictLocal(r)
	}
	seen := setup(t, Options{}).a
	seen.localNets.Store(nets)
	seen.skipLocal.Store(true)
	try(seen, "172.17.0.1", "8.8.8.8") // the proxy forwards once
	if try(seen, "172.17.0.1", "") {
		t.Error("the proxy seen forwarding skipped sign-in without headers")
	}
	if !try(seen, "192.168.1.20", "") {
		t.Error("a plain home visit no longer skips")
	}
	listed := setup(t, Options{TrustedProxies: []string{"172.17.0.1"}}).a
	listed.localNets.Store(nets)
	listed.skipLocal.Store(true)
	if try(listed, "172.17.0.1", "") {
		t.Error("a trusted proxy with no forwarding headers skipped sign-in")
	}
	if !try(listed, "192.168.1.20", "") {
		t.Error("a plain home visit no longer skips behind a listed proxy")
	}
}

// A call HolyArrs sends itself (a hook pointed back at it) never skips sign-in.
func TestSkipLocalNeverForSelfCalls(t *testing.T) {
	nets, err := parseLocalNets([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	a := setup(t, Options{}).a
	a.localNets.Store(nets)
	a.skipLocal.Store(true)
	r := httptest.NewRequest("GET", "/api/v1/titles", nil)
	r.RemoteAddr, r.Host = "10.0.0.5:5000", "10.0.0.9:6262"
	if !a.strictLocal(r) {
		t.Fatal("a plain home visit did not skip")
	}
	r.Header.Set(outbound.SelfCallHeader, "1")
	if a.strictLocal(r) {
		t.Fatal("a call HolyArrs sent itself skipped sign-in")
	}
}
