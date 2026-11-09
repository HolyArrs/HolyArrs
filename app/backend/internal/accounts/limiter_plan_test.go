package accounts

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A forged X-Real-IP never charges the real address it names: the owner who
// signed in from there before still gets in.
func TestForgedRealIPCannotLockOutOwnerAddress(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, f, client(t))
	owner := "203.0.113.50"
	right := `{"name":"luis","password":"` + pw + `"}`
	wrong := `{"name":"luis","password":"wrong password"}`
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", right, map[string]string{"X-Forwarded-For": owner}); resp.StatusCode != 200 {
		t.Fatalf("first sign-in: %d", resp.StatusCode)
	}
	for i := 0; i < 30; i++ {
		f.clock.add(MaxBackoff + time.Second)
		call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", wrong,
			map[string]string{"X-Forwarded-For": "198.51.100.66", "X-Real-IP": owner})
	}
	f.a.mu.Lock()
	held := f.a.fails["ip:"+owner] != nil
	f.a.mu.Unlock()
	if held {
		t.Error("forged X-Real-IP was counted under the ip: namespace")
	}
	if resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", right, map[string]string{"X-Forwarded-For": owner}); resp.StatusCode != 200 {
		t.Fatalf("owner held out by a forged X-Real-IP: %d %s", resp.StatusCode, body)
	}
}

// Rotating the forged X-Real-IP buys no fresh budget: the other header (the
// attacker's real address) pays on sign-in, single sign-on and Plex PINs.
func TestForgedRealIPRotationPaysRealKey(t *testing.T) {
	var pinID atomic.Int64
	plex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%d,"code":"abcd"}`, pinID.Add(1))
	}))
	t.Cleanup(plex.Close)
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}, PlexURL: plex.URL})
	setupOwner(t, f, client(t))
	attacker := "198.51.100.66"
	hdr := func(i int) map[string]string {
		return map[string]string{"X-Forwarded-For": attacker, "X-Real-IP": "192.0.2." + strconv.Itoa(i+1)}
	}
	got429 := -1
	for i := 0; i < freeTries+2; i++ {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"made-up-`+strconv.Itoa(i)+`","password":"wrong password"}`, hdr(i))
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = i
			break
		}
	}
	if got429 != freeTries {
		t.Errorf("login: first 429 at try %d, want %d", got429, freeTries)
	}
	got429 = -1
	for i := 0; i < freeTries+2; i++ {
		r := reqFrom("127.0.0.1:4000", [][2]string{{"X-Forwarded-For", attacker}, {"X-Real-IP", "192.0.2." + strconv.Itoa(100+i)}})
		if !f.a.ssoAttempt(httptest.NewRecorder(), r) {
			got429 = i
			break
		}
	}
	if got429 != freeTries {
		t.Errorf("sso: first refusal at try %d, want %d", got429, freeTries)
	}
	for i := 0; i < pinsPerIP+1; i++ {
		r := reqFrom("127.0.0.1:4000", [][2]string{{"X-Forwarded-For", attacker}, {"X-Real-IP", "192.0.2." + strconv.Itoa(200+i)}})
		w := httptest.NewRecorder()
		f.a.startPin(w, r, "")
		want := http.StatusOK
		if i == pinsPerIP {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Errorf("pin %d: %d, want %d", i+1, w.Code, want)
		}
	}
}

// Someone else signing in at a shared address takes back only their own
// reservation: the strangers' count there stays, so a member's sign-in never
// hands out fresh owner guesses.
func TestMemberSignInDoesNotClearSharedAddress(t *testing.T) {
	for _, way := range []string{"password", "jellyfin"} {
		t.Run(way, func(t *testing.T) {
			var logouts atomic.Int32
			ms := fakeMediaServer(t, &logouts)
			f := gated(t, Options{})
			own := ownerOf(t, f)
			right := `{"name":"luis","password":"` + pw + `"}`
			wrong := `{"name":"luis","password":"wrong password"}`
			if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", right, nil); resp.StatusCode != 200 {
				t.Fatalf("owner sign-in: %d", resp.StatusCode)
			}
			member := func() int { t.Fatal("no member way"); return 0 }
			switch way {
			case "password":
				join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
				member = func() int {
					resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"ana","password":"`+pw+`"}`, nil)
					return resp.StatusCode
				}
			case "jellyfin":
				own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"`+ms.URL+`/","join":"everyone"},"confirm_everyone":true}`)
				member = func() int {
					resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/media-server/jellyfin", `{"name":"mum","password":"right"}`, nil)
					return resp.StatusCode
				}
			}
			if code := member(); code != 200 {
				t.Fatalf("member first sign-in: %d", code)
			}
			checked := 0
			for round := 0; round < 50; round++ {
				held := false
				for i := 0; i < 4; i++ {
					resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", wrong, nil)
					if resp.StatusCode == http.StatusTooManyRequests {
						held = true
					} else {
						checked++
					}
				}
				if round >= 1 && !held {
					t.Fatalf("round %d: no wait for owner guesses", round+1)
				}
				member()
			}
			if checked > 6 {
				t.Fatalf("%d owner guesses checked, want at most 6", checked)
			}
		})
	}
}

// The code step of an outside sign-in (single sign-on, Plex, Jellyfin) is
// not held by the local password name's wait: it reads the first step's key.
func TestOutsideTwoFactorNotHeldByLocalNameWait(t *testing.T) {
	f := gated(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	own := ownerOf(t, f)
	secret, _ := turnOnTwoFactor(t, f, own)
	var id int64
	if err := f.s.Read().QueryRow("SELECT id FROM users WHERE name = 'luis'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	home := map[string]string{"X-Forwarded-For": "203.0.113.50"}
	for _, c := range []struct{ method, userKey string }{
		{"oidc", ""},
		{ProviderPlex, ""},
		{ProviderJellyfin, "name:jellyfin:luis"},
	} {
		f.clock.add(totpStep * time.Second)
		tok := token(32)
		f.a.mu.Lock()
		f.a.tickets[sum(tok)] = &ticket{userKey: c.userKey, userID: id, name: "luis", method: c.method, expires: f.clock.t.Add(ticketLife)}
		f.a.fails["name:luis"] = &failure{n: 31, last: f.clock.t}
		f.a.mu.Unlock()
		resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login/two-factor", `{"ticket":"`+tok+`","code":"`+codeNow(t, f, secret, 0)+`"}`, home)
		if resp.StatusCode != 200 {
			t.Errorf("%s: right code held by the local name wait: %d %s", c.method, resp.StatusCode, out)
		}
	}
}

// flood notes n failures, each from a fresh /64 under a made-up name; the
// caller holds a.mu.
func flood(a *Auth, tag string, n int, step time.Duration, clock *fakeClock) {
	for i := 0; i < n; i++ {
		if step > 0 {
			clock.add(step)
		}
		a.noteFail("ip:2001:db8:"+strconv.FormatInt(int64(i/65536), 16)+":"+strconv.FormatInt(int64(i%65536), 16)+"::/64", "name:made-up-"+tag+"-"+strconv.Itoa(i))
	}
}

// A real account's name count survives quiet periods and floods of made-up
// names, so each cycle of guessing does not start again from free guesses.
func TestPruneKeepsAccountCounts(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	for cycle := 0; cycle < 4; cycle++ {
		free := 0
		for g := 0; g < 25; g++ {
			if w := a.waitFor("name:luis", freeUserTries); w > 0 {
				f.clock.add(w)
			}
			if f := a.fails["name:luis"]; f == nil || f.n < freeUserTries {
				free++
			}
			a.noteFail("ip:198.51.100."+strconv.Itoa(g+1), "name:luis")
		}
		if cycle > 0 && free > 0 {
			t.Errorf("cycle %d: %d free guesses again", cycle+1, free)
		}
		f.clock.add(16 * time.Minute)
		flood(a, strconv.Itoa(cycle), 5100, 0, f.clock)
		if r := a.fails["name:luis"]; r == nil || r.n < 25 {
			t.Fatalf("cycle %d: owner's name count wiped by the flood: %+v", cycle+1, r)
		}
	}
}

// Per-account confirm, password-change and code counts survive floods.
func TestFloodKeepsReauthAndCodeCounts(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	keys := []string{"reauth:1", "pw:1", "code:1"}
	for i := 0; i < 20; i++ {
		a.noteFail(keys...)
	}
	f.clock.add(time.Minute)
	flood(a, "a", 5000, 10*time.Millisecond, f.clock)
	flood(a, "b", 3000, 10*time.Millisecond, f.clock)
	for _, k := range keys {
		if a.fails[k] == nil || a.waitFor(k, freeTries) == 0 {
			t.Errorf("%s: count lost to the flood", k)
		}
	}
}

// Made-up names still go after their wait, and a big flood stays bounded.
func TestMadeUpNamesStillPruned(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := 0; i < 100; i++ {
		a.noteFail("name:nobody-" + strconv.Itoa(i))
	}
	f.clock.add(16 * time.Minute)
	a.pruneFails(f.clock.t)
	for i := 0; i < 100; i++ {
		if a.fails["name:nobody-"+strconv.Itoa(i)] != nil {
			t.Fatal("made-up name kept after its wait")
		}
	}
	for i := 0; i < 50000; i++ {
		a.noteFail("ip:2001:db8:"+strconv.FormatInt(int64(i/65536), 16)+":"+strconv.FormatInt(int64(i%65536), 16)+"::/64", "name:big-"+strconv.Itoa(i))
		if len(a.fails) > 4*maxTracked {
			t.Fatalf("table grew to %d", len(a.fails))
		}
	}
}

// Known places are kept in the database, so home still counts after a
// restart while strangers hold the name's wait.
func TestKnownPlaceSurvivesRestart(t *testing.T) {
	opt := Options{TrustedProxies: []string{"127.0.0.1/32"}}
	f := setup(t, opt)
	setupOwner(t, f, client(t))
	right := `{"name":"luis","password":"` + pw + `"}`
	wrong := `{"name":"luis","password":"wrong password"}`
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", right, map[string]string{"X-Forwarded-For": "203.0.113.50"}); resp.StatusCode != 200 {
		t.Fatalf("first sign-in: %d", resp.StatusCode)
	}
	// A restart: a new Auth on the same database.
	opt.Now, opt.fastHash = f.clock.now, true
	a := New(f.s, opt)
	t.Cleanup(a.Close)
	m := http.NewServeMux()
	a.Routes(m)
	srv := httptest.NewServer(a.Middleware(m))
	t.Cleanup(srv.Close)
	login := func(from, body string) int {
		resp, _ := call(t, client(t), "POST", srv.URL+"/api/v1/auth/login", body, map[string]string{"X-Forwarded-For": from})
		return resp.StatusCode
	}
	for i := 0; i < 30; i++ {
		login("198.51.100."+strconv.Itoa(i%6+1), wrong)
	}
	if got := login("192.0.2.77", right); got != http.StatusTooManyRequests {
		t.Fatalf("owner from a never-used address: %d, want 429", got)
	}
	if got := login("203.0.113.50", right); got != http.StatusOK {
		t.Fatalf("owner from home after a restart: %d, want 200", got)
	}
}

// Forwarding headers from a private peer that is not trusted are ignored,
// and the log says so (once per 15 minutes) with the fix.
func TestUntrustedProxyHeadersWarnOnce(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	clock := newClock()
	a := &Auth{now: clock.now}
	lines := func() int { return strings.Count(buf.String(), "HOLYARRS_TRUSTED_PROXIES=172.18.0.2") }
	for i := 0; i < 3; i++ {
		a.ClientKey(reqFrom("172.18.0.2:4000", [][2]string{{"X-Forwarded-For", "203.0.113.9"}}))
	}
	if n := lines(); n != 1 {
		t.Fatalf("%d warnings, want 1: %s", n, buf.String())
	}
	clock.add(16 * time.Minute)
	a.ClientKey(reqFrom("172.18.0.2:4000", [][2]string{{"X-Forwarded-For", "203.0.113.9"}}))
	if n := lines(); n != 2 {
		t.Fatalf("%d warnings after 16 minutes, want 2", n)
	}
	buf.Reset()
	clock.add(16 * time.Minute)
	a.ClientKey(reqFrom("198.51.100.4:4000", [][2]string{{"X-Forwarded-For", "203.0.113.9"}}))
	if buf.Len() != 0 {
		t.Fatalf("warning for a public peer: %s", buf.String())
	}
}

// A typed name shaped like a media-server key ("x1:luis") is not a real
// account, and a saturated table takes only a bounded number of real-name
// records, so such a flood cannot grow the map without end.
func TestFakeKindNamesStayBounded(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := 0; i < 3*maxTracked; i++ {
		a.noteFail("name:filler-" + strconv.Itoa(i))
	}
	base := len(a.fails)
	for i := 0; i < 5000; i++ {
		a.noteFail("name:x" + strconv.Itoa(i) + ":luis")
	}
	if grew := len(a.fails) - base; grew > fullExtra {
		t.Fatalf("saturated table grew by %d records", grew)
	}
	if a.isRealName("name:x1:luis", f.clock.t) {
		t.Error("made-up kind counted as a real account")
	}
	for _, k := range []string{"name:luis", "name:jellyfin:luis", "name:emby:luis"} {
		if !a.isRealName(k, f.clock.t) {
			t.Errorf("%s not counted as real", k)
		}
	}
}
