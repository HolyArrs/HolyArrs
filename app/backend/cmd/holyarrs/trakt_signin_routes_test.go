package main

import (
	"strings"
	"testing"
)

// The Trakt sign-in belongs to the owner, like the Trakt VIP token: its
// token reads private lists and watch history. An admin with the settings
// permission may see whether someone is signed in, never a token, but may
// not start, check, replace or end the sign-in, or change the app secret.
//
// route: PUT /api/v1/settings/trakt-secret
// route: GET /api/v1/settings/trakt-signin
// route: POST /api/v1/settings/trakt-signin
// route: POST /api/v1/settings/trakt-signin/check
// route: DELETE /api/v1/settings/trakt-signin
func TestTraktSignInRoutesAreTheOwners(t *testing.T) {
	t.Parallel()
	r := newArrAskRig(t)
	r.owner.call("PUT", "/api/v1/users/"+arrItoa(r.bo.id), `{"role":"admin"}`, 200)
	r.owner.call("PUT", "/api/v1/users/"+arrItoa(r.bo.id), `{"perms":["ask","manage_library","settings"]}`, 200)
	r.owner.call("PUT", "/api/v1/settings/trakt-key", `{"key":"client-id-for-tests"}`, 200)
	changes := []struct{ method, path, body string }{
		{"PUT", "/api/v1/settings/trakt-secret", `{"secret":"secret-for-tests"}`},
		{"POST", "/api/v1/settings/trakt-signin", `{}`},
		{"POST", "/api/v1/settings/trakt-signin/check", `{}`},
		{"DELETE", "/api/v1/settings/trakt-signin", ``},
	}
	for _, c := range changes {
		r.bo.call(c.method, c.path, c.body, 403)  // an admin
		r.ana.call(c.method, c.path, c.body, 403) // a member
	}
	b := string(r.bo.call("GET", "/api/v1/settings/trakt-signin", "", 200))
	if !strings.Contains(b, `"signed_in":false`) || strings.Contains(b, "client-id-for-tests") {
		t.Fatalf("admin status %s", b)
	}
	r.ana.call("GET", "/api/v1/settings/trakt-signin", "", 403)
	// The owner may: a check with no sign-in started is refused; signing
	// out of nothing is fine.
	r.owner.call("PUT", "/api/v1/settings/trakt-secret", `{"secret":""}`, 200)
	r.owner.call("POST", "/api/v1/settings/trakt-signin/check", `{}`, 409)
	r.owner.call("DELETE", "/api/v1/settings/trakt-signin", ``, 200)
}
