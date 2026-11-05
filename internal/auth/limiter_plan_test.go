package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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
