package accounts

import (
	"net/http"
	"strings"
	"testing"
)

// A visit let in by the skip cannot change sign-in settings or confirm a
// password, and the refusal says where to go; a real password sign-in from
// that same browser then turns the skip off.
func TestLocalVisitorGetsToPasswordSignIn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	localHome(t, f, own, "")
	v, _ := localVisitor(t, f)

	resp, out := v.do(t, f, http.MethodPut, "/api/v1/auth/settings", `{"skip_login_local_network":false}`)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(out, `"reauth"`) {
		t.Fatalf("local visit changed sign-in settings: %d %s", resp.StatusCode, out)
	}
	resp, out = v.do(t, f, http.MethodPost, "/api/v1/auth/reauth", `{"password":"`+pw+`"}`)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(out, "sign-in page") {
		t.Fatalf("reauth from a local visit: %d %s", resp.StatusCode, out)
	}

	resp, out = call(t, v.c, http.MethodPost, f.srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("password sign-in from a local visit: %d %s", resp.StatusCode, out)
	}
	var s struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &s)
	signed := person{v.c, s.CSRF}
	if resp, out = signed.do(t, f, http.MethodPut, "/api/v1/auth/settings", `{"skip_login_local_network":false}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("skip off after password sign-in: %d %s", resp.StatusCode, out)
	}
}
