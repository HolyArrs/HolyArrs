package accounts

import (
	"fmt"
	"testing"
)

// route: POST /api/v1/users/{id}/password
func TestOwnerSetsForgottenPassword(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{}`), "ana")
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "bo")
	pw2 := pw + "-pw2"
	body := func(p string) string { return `{"password":"` + p + `"}` }
	path := fmt.Sprintf("/api/v1/users/%d/password", userID(t, f, "ana"))
	if resp, _ := own.do(t, f, "POST", path, body("x")); resp.StatusCode != 400 {
		t.Fatalf("short password taken: %d", resp.StatusCode)
	}
	if resp, out := own.do(t, f, "POST", path, body(pw2)); resp.StatusCode != 204 {
		t.Fatalf("owner set: %d %s", resp.StatusCode, out)
	}
	if resp, _ := ana.do(t, f, "GET", "/api/v1/auth/me", ""); resp.StatusCode != 401 {
		t.Fatalf("old session still works: %d", resp.StatusCode)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"ana","password":"`+pw+`"}`, nil); resp.StatusCode == 200 {
		t.Fatal("old password still works")
	}
	if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"ana","password":"`+pw2+`"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("new password: %d %s", resp.StatusCode, out)
	}
	// An admin may help a member but not the owner, another admin or themself.
	if resp, _ := ad.do(t, f, "POST", path, body(pw2+"2")); resp.StatusCode != 204 {
		t.Fatalf("admin on member: %d", resp.StatusCode)
	}
	for _, name := range []string{"luis", "bo"} {
		p := fmt.Sprintf("/api/v1/users/%d/password", userID(t, f, name))
		if resp, _ := ad.do(t, f, "POST", p, body(pw2+"2")); resp.StatusCode != 403 {
			t.Fatalf("admin on %s: %d", name, resp.StatusCode)
		}
	}
	// Nor a member who may do more than the admin.
	if resp, _ := own.do(t, f, "PUT", path[:len(path)-len("/password")], `{"perms":["ask","settings"]}`); resp.StatusCode != 200 {
		t.Fatalf("give ana settings: %d", resp.StatusCode)
	}
	if resp, _ := ad.do(t, f, "POST", path, body(pw2+"3")); resp.StatusCode != 403 {
		t.Fatalf("admin on a member with more: %d", resp.StatusCode)
	}
	if resp, _ := ana.do(t, f, "POST", path, body(pw2+"2")); resp.StatusCode != 401 && resp.StatusCode != 403 {
		t.Fatalf("member allowed: %d", resp.StatusCode)
	}
}
