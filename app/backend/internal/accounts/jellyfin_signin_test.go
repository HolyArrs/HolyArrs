package accounts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestJellyfinJoinRulesAndOff: each first-time join rule does what its label
// says, a linked person still gets in under "only people already linked",
// a server that is gone gives a plain message while password sign-in works,
// and turning the way off hides it and refuses it.
func TestJellyfinJoinRulesAndOff(t *testing.T) {
	var logouts atomic.Int32
	ms := fakeMediaServer(t, &logouts)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	login := func(name string) (int, string) {
		t.Helper()
		resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/media-server/jellyfin", `{"name":"`+name+`","password":"right"}`, nil)
		return resp.StatusCode, out
	}
	set := func(body string) {
		t.Helper()
		if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", body); resp.StatusCode != 200 {
			t.Fatalf("settings %s: %d %s", body, resp.StatusCode, out)
		}
	}
	way := `{"jellyfin":{"on":true,"url":"` + ms.URL + `",`

	// Only people already linked: a new person is refused with a plain message.
	set(way + `"join":"nobody"}}`)
	if code, out := login("mum"); code != 403 || !strings.Contains(out, "not allowed in yet") {
		t.Fatalf("nobody rule let a new person in: %d %s", code, out)
	}
	// Only the names on the list: Mum is listed, Kid is not.
	set(way + `"join":"list","allow":"mum, someone"}}`)
	if code, out := login("kid"); code != 403 {
		t.Fatalf("list rule let an unlisted person in: %d %s", code, out)
	}
	if code, out := login("mum"); code != 200 {
		t.Fatalf("list rule refused a listed person: %d %s", code, out)
	}
	// Back to only people already linked: Mum is linked now and still gets in.
	set(way + `"join":"nobody"}}`)
	if code, out := login("mum"); code != 200 {
		t.Fatalf("linked person refused: %d %s", code, out)
	}
	// Everyone needs confirming, then anyone the server lets in joins as a member.
	if resp, _ := own.do(t, f, "PUT", "/api/v1/auth/settings", way+`"join":"everyone"}}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("everyone without confirming: %d", resp.StatusCode)
	}
	set(way + `"join":"everyone"},"confirm_everyone":true}`)
	if code, out := login("kid"); code != 200 {
		t.Fatalf("everyone rule refused: %d %s", code, out)
	}

	// The server is gone: a plain message, and password sign-in still works.
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	set(`{"jellyfin":{"on":true,"url":"` + gone.URL + `","join":"everyone"}}`)
	if code, out := login("mum"); code != http.StatusBadGateway || !strings.Contains(out, "did not answer") {
		t.Fatalf("server gone: %d %s", code, out)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("password sign-in while the server is gone: %d", resp.StatusCode)
	}

	// Off: the sign-in page no longer offers it and the server refuses it.
	set(`{"jellyfin":{"on":false,"url":"` + ms.URL + `","join":"everyone"}}`)
	if _, st := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil); strings.Contains(st, `"jellyfin"`) {
		t.Fatalf("state still offers jellyfin: %s", st)
	}
	if code, _ := login("mum"); code != 404 {
		t.Fatalf("sign-in while off: %d", code)
	}
}
