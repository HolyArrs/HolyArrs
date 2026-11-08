package accounts

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestLimitAddrGroupsIPv6(t *testing.T) {
	same := [][2]string{
		{"2001:db8:1:2::1", "2001:db8:1:2:ffff:ffff:ffff:ffff"},
		{"::ffff:198.51.100.7", "198.51.100.7"},
	}
	for _, p := range same {
		if limitAddr(p[0]) != limitAddr(p[1]) {
			t.Errorf("%s and %s should share a key: %s %s", p[0], p[1], limitAddr(p[0]), limitAddr(p[1]))
		}
	}
	if limitAddr("2001:db8:1:2::1") == limitAddr("2001:db8:1:3::1") {
		t.Error("different /64 networks share a key")
	}
	if limitAddr("198.51.100.7") == limitAddr("198.51.100.8") {
		t.Error("different IPv4 addresses share a key")
	}
}

// One /64 owns many addresses; the per-address budget is for the network.
func TestIPv6NetworkSharesAddressBudget(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, f, client(t))
	wrong := `{"name":"nobody","password":"wrong password"}`
	got := 0
	for i := 0; i < freeTries+2; i++ {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", wrong, map[string]string{"X-Forwarded-For": "2001:db8:5:5::" + strconv.Itoa(i+1)})
		got = resp.StatusCode
	}
	if got != http.StatusTooManyRequests {
		t.Fatalf("fresh address in the same /64 after %d wrong tries: %d, want 429", freeTries+1, got)
	}
}

// Flooding fresh addresses must not flush the wait that protects a name.
func TestFloodKeepsNameWait(t *testing.T) {
	f := setup(t, Options{})
	a := f.a
	a.mu.Lock()
	for i := 0; i < freeUserTries+3; i++ {
		a.noteFail("ip:198.51.100.1", "name:luis")
	}
	a.mu.Unlock()
	for i := 0; i < 20000; i++ {
		a.mu.Lock()
		a.noteFail("ip:" + strconv.Itoa(i))
		a.mu.Unlock()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.waitFor("name:luis", freeUserTries) == 0 {
		t.Fatal("name wait flushed by a flood of fresh addresses")
	}
	if len(a.fails) > 2*maxTracked {
		t.Fatalf("fail map grew to %d", len(a.fails))
	}
	// Expired records go first.
	f.clock.add(time.Hour)
	for i := 0; i < maxTracked; i++ {
		a.fails["ip:x"+strconv.Itoa(i)] = &failure{n: 1, last: a.now().Add(-time.Hour)}
	}
	a.noteFail("ip:again")
	if len(a.fails) > maxTracked/2 {
		t.Fatalf("expired records kept: %d", len(a.fails))
	}
}

// Other names and addresses cannot push a name's known places out.
func TestFloodKeepsKnownPlace(t *testing.T) {
	f := setup(t, Options{})
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	a.noteKnown(place{"ip:203.0.113.50", "name:luis", true})
	for i := 0; i < 5000; i++ {
		a.noteKnown(place{"ip:" + strconv.Itoa(i), "name:other" + strconv.Itoa(i%1000), true})
		a.noteKnown(place{"ip:" + strconv.Itoa(i), "name:flood" + strconv.Itoa(i%400), true})
	}
	for i := 0; i < freeUserTries+3; i++ {
		a.noteFail("name:luis")
	}
	if a.signInWait("ip:203.0.113.50", "name:luis", true) != 0 {
		t.Fatal("owner's known place evicted by other names")
	}
	if a.signInWait("ip:203.0.113.50", "name:luis", false) == 0 {
		t.Fatal("an unsure address skipped the name wait")
	}
	for i := 0; i < 100; i++ {
		a.noteKnown(place{"ip:own" + strconv.Itoa(i), "name:luis", true})
	}
	if len(a.known["name:luis"]) > knownPerName {
		t.Fatalf("one name keeps %d places", len(a.known["name:luis"]))
	}
	a.noteKnown(place{"ip:198.51.100.99", "name:luis", false})
	if _, ok := a.known["name:luis"]["ip:198.51.100.99"]; ok {
		t.Fatal("an unsure address was recorded as a known place")
	}
}

// nginx with only X-Real-IP passes the visitor's X-Forwarded-For through, so
// a visitor can write the owner's home address there. It never counts as a
// known place, and a sign-in through it records none.
func TestDisagreeingHeadersNeverKnownPlace(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, f, client(t))
	right := `{"name":"luis","password":"` + pw + `"}`
	wrong := `{"name":"luis","password":"wrong password"}`
	login := func(hdr map[string]string, body string) int {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", body, hdr)
		return resp.StatusCode
	}
	home := map[string]string{"X-Forwarded-For": "203.0.113.50"}
	forged := map[string]string{"X-Forwarded-For": "203.0.113.50", "X-Real-IP": "192.0.2.99"}
	if got := login(home, right); got != http.StatusOK {
		t.Fatalf("home: %d", got)
	}
	// A sign-in through disagreeing headers records nothing.
	other := map[string]string{"X-Forwarded-For": "203.0.113.77", "X-Real-IP": "192.0.2.98"}
	if got := login(other, right); got != http.StatusOK {
		t.Fatalf("other: %d", got)
	}
	for i := 0; i < freeUserTries+2; i++ {
		login(map[string]string{"X-Forwarded-For": "198.51.100." + strconv.Itoa(i+1)}, wrong)
	}
	if got := login(forged, right); got != http.StatusTooManyRequests {
		t.Fatalf("forged known-place address skipped the name wait: %d, want 429", got)
	}
	if got := login(other, right); got != http.StatusTooManyRequests {
		t.Fatalf("a sign-in through disagreeing headers became a known place: %d, want 429", got)
	}
	if got := login(home, right); got != http.StatusOK {
		t.Fatalf("real home during the name wait: %d, want 200", got)
	}
}

// A good sign-in or an undone attempt on another name never clears a name's wait.
func TestOtherNameDoesNotClearNameWait(t *testing.T) {
	f := gated(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{"days":1}`), "ana")
	wrong := `{"name":"luis","password":"wrong password"}`
	login := func(from, body string) int {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", body, map[string]string{"X-Forwarded-For": from})
		return resp.StatusCode
	}
	for i := 0; i < freeUserTries+1; i++ {
		login("198.51.100."+strconv.Itoa(i+1), wrong)
	}
	if got := login("192.0.2.1", `{"name":"ana","password":"`+pw+`"}`); got != http.StatusOK {
		t.Fatalf("ana: %d", got)
	}
	if got := login("192.0.2.2", wrong); got != http.StatusTooManyRequests {
		t.Fatalf("ana's sign-in cleared luis's wait: %d, want 429", got)
	}
	a := f.a
	a.mu.Lock()
	marks := a.markFails("ip:192.0.2.9", "name:jellyfin:ana")
	a.noteFail("ip:192.0.2.9", "name:jellyfin:ana")
	a.mu.Unlock()
	a.undoFail(marks)
	if got := login("192.0.2.3", wrong); got != http.StatusTooManyRequests {
		t.Fatalf("undoFail for ana cleared luis's wait: %d, want 429", got)
	}
}

func TestNameHiddenFillersAndSpaces(t *testing.T) {
	for _, n := range []string{"lu️is", "lu\U000e0100is", "lu͏is", "luᅟis", "luᅠis", "luㅤis", "luﾠis", "lu⠀is", "lu᠋is", "lu᠏is", "lu឴is", "lu឵is",
		"️", "a️️", " luis", "luis ", "lu  is"} {
		if nameProblem(n) == "" {
			t.Errorf("%q accepted", n)
		}
	}
	for _, n := range []string{"❤️", "I ❤️ you", "lu is"} {
		if msg := nameProblem(n); msg != "" {
			t.Errorf("%q refused: %s", n, msg)
		}
	}
}

// Flooding made-up names must not evict the owner's active name wait, and the
// map stays bounded. Wait behaviour must not depend on the name existing.
func TestNameFloodKeepsOwnerWait(t *testing.T) {
	f := setup(t, Options{})
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := 0; i < freeUserTries+3; i++ {
		a.noteFail("ip:198.51.100.1", "name:luis")
	}
	f.clock.add(time.Second) // the owner's record is now the oldest
	for i := 0; i < 50000; i++ {
		a.noteFail("ip:"+strconv.Itoa(i%300), "name:made-up-"+strconv.Itoa(i))
	}
	if a.waitFor("name:luis", freeUserTries) == 0 {
		t.Fatal("owner name wait evicted by a flood of made-up names")
	}
	if len(a.fails) > 4*maxTracked {
		t.Fatalf("fail map grew to %d", len(a.fails))
	}
	// The address key still counts when a name record is not added.
	a.noteFail("ip:203.0.113.9", "name:one-more")
	if a.fails["ip:203.0.113.9"] == nil {
		t.Fatal("address attempt not counted while the name table was full")
	}
}

// While the name table is full, a made-up name gets no record, but a real
// account still gets its own per-name wait; a flood holds nobody else.
func TestSaturatedNameTableProtectsRealNames(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{"days":1}`), "ana")
	join(t, f, invite(t, f, own, `{"days":1}`), "bob")
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := 0; i < 4*maxTracked; i++ {
		a.noteFail("ip:"+strconv.Itoa(i%300), "name:made-up-"+strconv.Itoa(i))
	}
	if a.fails["name:ana"] != nil {
		t.Fatal("test needs ana untracked")
	}
	a.noteKnown(place{"ip:203.0.113.50", "name:ana", true})
	f.clock.add(2 * time.Second)

	// (a) guesses at a real name from fresh addresses build its own wait.
	for i := 0; i < freeUserTries; i++ {
		ip := "ip:192.0.2." + strconv.Itoa(i+1)
		if w := a.signInWait(ip, "name:ana", true); w != 0 {
			t.Fatalf("guess %d waited %v", i, w)
		}
		a.noteFail(ip, "name:ana")
	}
	if a.signInWait("ip:198.51.100.1", "name:ana", true) == 0 {
		t.Fatal("real name went free from a fresh address while the table was full")
	}
	// (b) a different real user is not held by the flood or by ana's wait.
	if w := a.signInWait("ip:198.51.100.2", "name:bob", true); w != 0 {
		t.Fatalf("bob held by the flood: %v", w)
	}
	// made-up names get no record and no shared wait.
	a.noteFail("ip:198.51.100.3", "name:never-seen")
	if a.fails["name:never-seen"] != nil {
		t.Fatal("made-up name got a record in a full table")
	}
	if w := a.signInWait("ip:198.51.100.4", "name:never-seen", true); w != 0 {
		t.Fatalf("made-up name waited %v", w)
	}
	// (c) the known place still gets in.
	if w := a.signInWait("ip:203.0.113.50", "name:ana", true); w != 0 {
		t.Fatalf("known place waited %v", w)
	}
	if len(a.fails) > 4*maxTracked+10 {
		t.Fatalf("fail map grew to %d", len(a.fails))
	}
}

// After a flood the prune schedule must not stay stuck: once every record has
// expired, a new name gets a real record again and saturation ends.
func TestSaturationEndsAfterFlood(t *testing.T) {
	f := setup(t, Options{})
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := 0; i < 5*maxTracked; i++ {
		a.noteFail("ip:"+strconv.Itoa(i), "name:made-up-"+strconv.Itoa(i))
	}
	f.clock.add(MaxBackoff + time.Minute)
	a.noteFail("ip:198.51.100.7", "name:fresh")
	if a.fails["name:fresh"] == nil {
		t.Fatal("new name got no record after the flood expired")
	}
	if len(a.fails) > maxTracked {
		t.Fatalf("expired records kept: %d", len(a.fails))
	}
}

// Disagreeing proxy headers: either one may be written by the visitor, so a
// failure counts under both keys and the larger wait stands.
func TestDisagreeingHeadersCountBothKeys(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	r, _ := http.NewRequest("POST", "/", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "203.0.113.50")
	r.Header.Set("X-Real-IP", "192.0.2.99")
	k1, k2, sure := f.a.clientKeys(r)
	if sure || k1 == "" || k2 == "" || k1 == k2 {
		t.Fatalf("keys %q %q sure %v", k1, k2, sure)
	}
	if k1 != "xri:192.0.2.99" || k2 != "xff:203.0.113.50" {
		t.Fatalf("keys %q %q", k1, k2)
	}
	r.Header.Set("X-Real-IP", "203.0.113.50")
	k1, k2, sure = f.a.clientKeys(r)
	if !sure || k2 != "" || k1 != "ip:203.0.113.50" {
		t.Fatalf("agreeing headers: %q %q %v", k1, k2, sure)
	}

	// Guesses with a changing X-Real-IP but one fixed X-Forwarded-For hit the wait.
	g := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, g, client(t))
	wrong := `{"name":"nobody","password":"wrong password"}`
	got := 0
	for i := 0; i < freeTries+2; i++ {
		resp, _ := call(t, client(t), "POST", g.srv.URL+"/api/v1/auth/login", wrong,
			map[string]string{"X-Forwarded-For": "203.0.113.50", "X-Real-IP": "192.0.2." + strconv.Itoa(i+1)})
		got = resp.StatusCode
	}
	if got != http.StatusTooManyRequests {
		t.Fatalf("rotating X-Real-IP dodged the limit: %d", got)
	}
}

// A visitor behind a proxy that sets only X-Real-IP can write any address in
// X-Forwarded-For. Those counts live in their own namespace and never lock a
// real address out of a sure request.
func TestForgedForwardedForCannotLockOutOwnerAddress(t *testing.T) {
	f := setup(t, Options{TrustedProxies: []string{"127.0.0.1/32"}})
	setupOwner(t, f, client(t))
	owner := "198.51.100.77"
	wrong := `{"name":"luis","password":"wrong password"}`
	for i := 0; i < freeTries+freeUserTries; i++ {
		resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", wrong,
			map[string]string{"X-Forwarded-For": owner, "X-Real-IP": "192.0.2." + strconv.Itoa(i%200+1)})
		_ = resp
	}
	f.a.mu.Lock()
	if f.a.fails["ip:"+owner] != nil {
		f.a.mu.Unlock()
		t.Fatal("forwarded address was counted under the ip: namespace")
	}
	f.a.mu.Unlock()
	resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"nobody","password":"wrong password"}`,
		map[string]string{"X-Forwarded-For": owner})
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Fatalf("owner's address held by forged forwarding: %d %s", resp.StatusCode, body)
	}
}

// Only the proxy the warning saw shares its address budget. A home device
// that writes its own X-Forwarded-For keeps the per-address limit.
func TestForgedForwardKeepsAddressLimit(t *testing.T) {
	a := setup(t, Options{}).a
	req := func(peer, xff string) *http.Request {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		r.RemoteAddr = peer + ":5000"
		r.Header.Set("X-Forwarded-For", xff)
		return r
	}
	if ip, _, _ := a.signInKeys(req("172.17.0.1", "203.0.113.7")); ip != "" {
		t.Fatalf("the seen proxy kept a shared address key %q", ip)
	}
	if ip, _, _ := a.signInKeys(req("192.168.1.50", "203.0.113.8")); ip != "ip:192.168.1.50" {
		t.Fatalf("a home device forging X-Forwarded-For got key %q, want ip:192.168.1.50", ip)
	}
}
