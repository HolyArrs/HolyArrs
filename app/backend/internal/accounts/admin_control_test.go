package accounts

import (
	"strconv"
	"testing"
)

// keyWith makes an owner's API key holding scopes and returns it.
func keyWith(t *testing.T, f *fixture, own person, scopes string) string {
	t.Helper()
	resp, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"k","scopes":[`+scopes+`]}`)
	if resp.StatusCode != 201 {
		t.Fatalf("make key %s: %d %s", scopes, resp.StatusCode, out)
	}
	var k struct {
		Key string `json:"key"`
	}
	jsonOf(t, out, &k)
	return k.Key
}

// TestAdminScopeCannotMakeAnAdmin: an admin-scope (or older all-scope) key
// would mint a session holding the control scope by inviting or promoting
// an admin. Making an admin needs the control scope or a session.
func TestAdminScopeCannotMakeAnAdmin(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	member := join(t, f, invite(t, f, own, `{}`), "pat")
	id := strconv.FormatInt(userID(t, f, "pat"), 10)
	_ = member
	bare := client(t)
	for _, scopes := range []string{`"admin"`, `"all"`, `"read","write","admin"`} {
		key := keyWith(t, f, own, scopes)
		h := map[string]string{APIKeyHeader: key}
		if resp, out := call(t, bare, "POST", f.srv.URL+"/api/v1/users/invites", `{"role":"admin"}`, h); resp.StatusCode != 403 {
			t.Errorf("key %s made an admin invite: %d %s", scopes, resp.StatusCode, out)
		}
		if resp, out := call(t, bare, "PUT", f.srv.URL+"/api/v1/users/"+id, `{"role":"admin"}`, h); resp.StatusCode != 403 {
			t.Errorf("key %s promoted a member to admin: %d %s", scopes, resp.StatusCode, out)
		}
		// Member invites stay allowed.
		if resp, out := call(t, bare, "POST", f.srv.URL+"/api/v1/users/invites", `{}`, h); resp.StatusCode != 201 {
			t.Errorf("key %s member invite: %d %s", scopes, resp.StatusCode, out)
		}
	}
	key := keyWith(t, f, own, `"admin","control"`)
	h := map[string]string{APIKeyHeader: key}
	if resp, out := call(t, bare, "POST", f.srv.URL+"/api/v1/users/invites", `{"role":"admin"}`, h); resp.StatusCode != 201 {
		t.Errorf("admin+control key admin invite: %d %s", resp.StatusCode, out)
	}
	if resp, out := call(t, bare, "PUT", f.srv.URL+"/api/v1/users/"+id, `{"role":"admin"}`, h); resp.StatusCode != 200 {
		t.Errorf("admin+control key promote: %d %s", resp.StatusCode, out)
	}
	// A session may.
	if resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"role":"admin"}`); resp.StatusCode != 201 {
		t.Errorf("session admin invite: %d %s", resp.StatusCode, out)
	}
}
