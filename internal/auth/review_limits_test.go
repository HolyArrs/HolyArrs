package auth

import (
	"net/http"
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
