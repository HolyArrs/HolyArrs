package accounts

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// localHome turns the skip on for loopback (where the test client is).
func localHome(t *testing.T, f *fixture, own person, extra string) {
	t.Helper()
	body := `{"skip_login_local_network":true,"local_networks":["127.0.0.1"]` + extra + `}`
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", body); resp.StatusCode != 200 {
		t.Fatalf("skip on: %d %s", resp.StatusCode, out)
	}
}

// localVisitor is a browser at home with no sign-in, holding the CSRF token
// /auth/me hands out for its local cookie.
func localVisitor(t *testing.T, f *fixture) (person, string) {
	t.Helper()
	c := client(t)
	resp, out := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
	if resp.StatusCode != 200 {
		return person{c: c}, out
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &v)
	return person{c, v.CSRF}, out
}

// Local visitors become the picked person, with that person's own role: a
// member cannot reach owner-only routes.
func TestLocalVisitorIsPickedPerson(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	ana := userID(t, f, "ana")
	localHome(t, f, own, `,"local_person":`+strconv.FormatInt(ana, 10))
	v, me := localVisitor(t, f)
	if !strings.Contains(me, `"name":"ana"`) || !strings.Contains(me, `"role":"member"`) || !strings.Contains(me, `"via":"local_network"`) {
		t.Fatalf("me: %s", me)
	}
	if v.csrf == "" {
		t.Fatalf("no CSRF token for a local visit: %s", me)
	}
	if resp, out := v.do(t, f, "GET", "/api/v1/users", ""); resp.StatusCode != 403 {
		t.Fatalf("member reached people: %d %s", resp.StatusCode, out)
	}
	_, out := own.do(t, f, "GET", "/api/v1/auth/settings", "")
	if !strings.Contains(out, `"local_person":`+strconv.FormatInt(ana, 10)) || !strings.Contains(out, `"local_skip_off":""`) {
		t.Fatalf("settings: %s", out)
	}
}

// The owner is the default; a picked person who is turned off or deleted
// ends the skip and is never replaced by the owner.
func TestLocalPersonGoneNeedsSignIn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	localHome(t, f, own, "")
	if _, me := localVisitor(t, f); !strings.Contains(me, `"name":"luis"`) {
		t.Fatalf("default is not the owner: %s", me)
	}
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	ana := strconv.FormatInt(userID(t, f, "ana"), 10)
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"local_person":`+ana+`}`); resp.StatusCode != 200 {
		t.Fatalf("pick: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/"+ana, `{"state":"disabled"}`); resp.StatusCode != 200 {
		t.Fatalf("disable: %d %s", resp.StatusCode, out)
	}
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("turned-off person still let in: %d", resp.StatusCode)
	}
	_, out := own.do(t, f, "GET", "/api/v1/auth/settings", "")
	if !strings.Contains(out, `"local_skip_off":"person_gone"`) {
		t.Fatalf("settings: %s", out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"local_person":`+ana+`}`); resp.StatusCode != 400 || !strings.Contains(out, "turned on") {
		t.Fatalf("picked a turned-off person: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"local_person":99999}`); resp.StatusCode != 400 {
		t.Fatalf("picked nobody: %d %s", resp.StatusCode, out)
	}
	// A deleted person (an id no one has) behaves the same.
	f.a.localPerson.Store(99999)
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("missing person let in: %d", resp.StatusCode)
	}
}

// A state-changing local visit needs the CSRF token of its own local cookie
// and the same-site check.
func TestLocalVisitNeedsCSRF(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	localHome(t, f, own, "")
	v, _ := localVisitor(t, f)
	post := func(hdr map[string]string) int {
		resp, _ := call(t, v.c, "POST", f.srv.URL+"/api/v1/titles", "{}", hdr)
		return resp.StatusCode
	}
	if got := post(nil); got != 403 {
		t.Fatalf("no token: %d", got)
	}
	if got := post(map[string]string{CSRFHeader: "nope"}); got != 403 {
		t.Fatalf("wrong token: %d", got)
	}
	if got := post(map[string]string{CSRFHeader: v.csrf, "Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}); got != 403 {
		t.Fatalf("cross-site post: %d", got)
	}
	if got := post(map[string]string{CSRFHeader: v.csrf}); got != 200 {
		t.Fatalf("right token: %d", got)
	}
	// Another browser's token does not fit this one's cookie, and a
	// browser with no local cookie yet cannot post at all.
	other, _ := localVisitor(t, f)
	if got := post(map[string]string{CSRFHeader: other.csrf}); got != 403 {
		t.Fatalf("another browser's token: %d", got)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/titles", "{}", map[string]string{CSRFHeader: v.csrf}); resp.StatusCode != 403 {
		t.Fatalf("no cookie: %d", resp.StatusCode)
	}
}

// A link from another site arrives without the SameSite=Strict local cookie,
// even when the browser has one. Making a new cookie there would change the
// token under the tabs already open, so their next save would fail; only a
// same-site visit makes one.
func TestCrossSiteVisitMakesNoLocalCookie(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	localHome(t, f, own, "")
	got := func(site string) bool {
		resp, out := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/me", "", map[string]string{"Sec-Fetch-Site": site})
		if resp.StatusCode != 200 {
			t.Fatalf("%s visit: %d %s", site, resp.StatusCode, out)
		}
		for _, ck := range resp.Cookies() {
			if ck.Name == LocalCookieName {
				return true
			}
		}
		return false
	}
	if got("cross-site") {
		t.Error("a cross-site visit made a new local cookie")
	}
	for _, site := range []string{"same-origin", "none", ""} {
		if !got(site) {
			t.Errorf("Sec-Fetch-Site %q: no local cookie", site)
		}
	}
}

// Kid guard: with a Kid around, only a Kid may be the person local visitors
// become; then more Kids may be made.
func TestLocalSkipKidGuard(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{"role":"member","max_rating":"PG"}`), "kid")
	kid := strconv.FormatInt(userID(t, f, "kid"), 10)
	body := `{"skip_login_local_network":true,"local_networks":["127.0.0.1"]}`
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", body); resp.StatusCode != 400 || !strings.Contains(out, "Kid account has an age limit") {
		t.Fatalf("owner with a Kid around: %d %s", resp.StatusCode, out)
	}
	localHome(t, f, own, `,"local_person":`+kid)
	if _, me := localVisitor(t, f); !strings.Contains(me, `"name":"kid"`) || !strings.Contains(me, `"max_rating":"PG"`) {
		t.Fatalf("me: %s", me)
	}
	// Another Kid is fine now: anyone at home is a Kid too.
	if resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"max_rating":"PG"}`); resp.StatusCode != 201 {
		t.Fatalf("kid invite while a Kid is the local person: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"local_person":0}`); resp.StatusCode != 400 || !strings.Contains(out, "age limit") {
		t.Fatalf("back to the owner with Kids around: %d %s", resp.StatusCode, out)
	}
	// Lifting the picked Kid's limit while another Kid invite is open ends the skip.
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/"+kid, `{"max_rating":""}`); resp.StatusCode != 200 {
		t.Fatalf("lift: %d %s", resp.StatusCode, out)
	}
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 401 {
		t.Fatalf("non-Kid let in with a Kid invite open: %d", resp.StatusCode)
	}
}

// With the owner as the local person, new Kids stay refused (today's rule).
func TestLocalSkipOwnerRefusesKids(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	localHome(t, f, own, "")
	if resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"max_rating":"PG"}`); resp.StatusCode != 400 || !strings.Contains(out, "skipping sign-in") {
		t.Fatalf("kid invite while the owner is the local person: %d %s", resp.StatusCode, out)
	}
}

// Plex, Jellyfin and Emby sign-in do not stop the skip; the settings name
// the ways that are on, for the card.
func TestLocalSkipWithMediaServerSignIn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	localHome(t, f, own, "")
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"http://192.168.1.10:8096","join":"nobody"}}`); resp.StatusCode != 200 {
		t.Fatalf("jellyfin on: %d %s", resp.StatusCode, out)
	}
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 200 {
		t.Fatalf("skip stopped with Jellyfin sign-in on: %d", resp.StatusCode)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/settings", ""); !strings.Contains(out, `"media_sign_in":["jellyfin"]`) {
		t.Fatalf("settings: %s", out)
	}
}

// "Add usual home ranges" never adds loopback or link-local, and the
// Tailscale and carrier range only when asked.
func TestAddHomeRanges(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	_, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"local_networks":["192.168.1.0/24"],"add_home_ranges":true}`)
	var v struct {
		Nets []string `json:"local_networks"`
	}
	jsonOf(t, out, &v)
	if got := strings.Join(v.Nets, ","); got != "192.168.1.0/24,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7" {
		t.Fatalf("home ranges: %s", got)
	}
	_, out = own.do(t, f, "PUT", "/api/v1/auth/settings", `{"add_cgnat":true}`)
	jsonOf(t, out, &v)
	if !strings.Contains(strings.Join(v.Nets, ","), "100.64.0.0/10") {
		t.Fatalf("cgnat not added: %v", v.Nets)
	}
	for _, n := range v.Nets {
		if strings.HasPrefix(n, "127.") || strings.HasPrefix(n, "169.254.") || strings.HasPrefix(n, "fe80") {
			t.Fatalf("added %s", n)
		}
	}
	for _, bad := range []string{"10.0.0/8", "1.0.0.0/4", "192.168.1.300"} {
		resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"local_networks":["`+bad+`"]}`)
		if resp.StatusCode != 400 || !strings.Contains(out, bad) {
			t.Errorf("%s: %d %s", bad, resp.StatusCode, out)
		}
	}
}

// Trusted proxies: saved ones are added to the fixed ones, every entry is
// checked, and the proxy this browser comes through cannot be dropped.
func TestTrustedProxiesInSettings(t *testing.T) {
	f := gated(t, Options{TrustedProxies: []string{"10.9.9.9"}})
	own := ownerOf(t, f)
	put := func(body string, hdr map[string]string) (int, string) {
		h := map[string]string{CSRFHeader: own.csrf}
		for k, v := range hdr {
			h[k] = v
		}
		resp, out := call(t, own.c, "PUT", f.srv.URL+"/api/v1/auth/settings", body, h)
		return resp.StatusCode, out
	}
	for _, bad := range []string{"10.9.9", "0.0.0.0/2", "proxy"} {
		if code, out := put(`{"trusted_proxies":["`+bad+`"]}`, nil); code != 400 || !strings.Contains(out, bad) {
			t.Errorf("%s: %d %s", bad, code, out)
		}
	}
	code, out := put(`{"trusted_proxies":[" 127.0.0.1 ",""]}`, nil)
	if code != 200 || !strings.Contains(out, `"trusted_proxies":["127.0.0.1/32"]`) || !strings.Contains(out, `"trusted_proxies_fixed":["10.9.9.9/32"]`) {
		t.Fatalf("save: %d %s", code, out)
	}
	// Now loopback's forwarded address is believed.
	localHome(t, f, own, `,"local_networks":["192.168.1.0/24"]`)
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", map[string]string{"X-Forwarded-For": "192.168.1.5"}); resp.StatusCode != 200 {
		t.Fatalf("trusted proxy's home client: %d", resp.StatusCode)
	}
	// Dropping the proxy this browser comes through is refused.
	if code, out := put(`{"trusted_proxies":[]}`, map[string]string{"X-Forwarded-For": "192.168.1.5"}); code != 409 || !strings.Contains(out, "127.0.0.1") {
		t.Fatalf("dropped own proxy: %d %s", code, out)
	}
	if code, out := put(`{"trusted_proxies":[]}`, nil); code != 200 || !strings.Contains(out, `"trusted_proxies_fixed":["10.9.9.9/32"]`) {
		t.Fatalf("clear: %d %s", code, out)
	}
	// Untrusted again: the header from loopback no longer counts.
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/titles", "", map[string]string{"X-Forwarded-For": "192.168.1.5"}); resp.StatusCode != 401 {
		t.Fatalf("untrusted peer's header believed: %d", resp.StatusCode)
	}
	// The fixed proxy still applies.
	if !f.a.trustedProxy(parseHostIP("10.9.9.9")) {
		t.Fatal("fixed proxy dropped")
	}
	// Saved proxies come back after a restart.
	put(`{"trusted_proxies":["172.18.0.2"]}`, nil)
	a := New(f.s, Options{fastHash: true})
	t.Cleanup(a.Close)
	if !a.trustedProxy(parseHostIP("172.18.0.2")) || a.trustedProxy(parseHostIP("10.9.9.9")) {
		t.Fatal("saved proxies not loaded, or fixed ones leaked into a new start")
	}
}

// The Host header is written by the visitor, so it never makes a peer local:
// a request from a container gateway counts only when the gateway itself is
// listed, whatever address the visitor says it typed.
func TestGatewayPeerNeedsItsOwnListing(t *testing.T) {
	a := setup(t, Options{}).a
	a.skipLocal.Store(true)
	try := func(list []string, peer, host string) bool {
		nets, err := parseLocalNets(list)
		if err != nil {
			t.Fatal(err)
		}
		a.localNets.Store(nets)
		r := httptest.NewRequest("GET", "/api/v1/titles", nil)
		r.RemoteAddr, r.Host = peer+":5000", host
		return a.strictLocal(r)
	}
	home := []string{"192.168.1.0/24"}
	for _, c := range []struct {
		list       []string
		peer, host string
		want       bool
	}{
		{home, "172.17.0.1", "192.168.1.9:6262", false}, // gateway, typed address listed
		{home, "172.20.0.1", "192.168.1.9", false},
		{home, "172.17.0.1", "[2001:db8::1]:6262", false},
		{home, "192.168.1.5", "192.168.1.9:6262", true},
		{home, "8.8.8.8", "192.168.1.9:6262", false},
		{[]string{"172.17.0.1"}, "172.17.0.1", "192.168.1.9:6262", true}, // listed on purpose, as today
	} {
		if got := try(c.list, c.peer, c.host); got != c.want {
			t.Errorf("%v: %s Host %s: %v, want %v", c.list, c.peer, c.host, got, c.want)
		}
	}
}
