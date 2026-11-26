package accounts

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// permsOf reads the stored permission list of the person named name.
func permsOf(t *testing.T, f *fixture, name string) string {
	t.Helper()
	var perms string
	if err := f.s.Read().QueryRow("SELECT perms FROM users WHERE name = ?", name).Scan(&perms); err != nil {
		t.Fatalf("perms of %s: %v", name, err)
	}
	return perms
}

func setNewPeople(t *testing.T, f *fixture, own person, list string) {
	t.Helper()
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"new_people_perms":`+list+`}`); resp.StatusCode != 200 {
		t.Fatalf("new people %s: %d %s", list, resp.StatusCode, out)
	}
}

// Out of the box nothing changes: new people start with requests only, and
// both the settings and the Users screen say so.
func TestNewPeopleStartWithRequestsOutOfTheBox(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if _, out := own.do(t, f, "GET", "/api/v1/auth/settings", ""); !strings.Contains(out, `"new_people_perms":["ask"]`) {
		t.Fatalf("settings: %s", out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/users", ""); !strings.Contains(out, `"new_people":["ask"]`) {
		t.Fatalf("users: %s", out)
	}
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	if _, me := ana.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(me, `"perms":["ask"]`) {
		t.Fatalf("me: %s", me)
	}
}

// Only a member's own permissions can be picked, and only by the owner:
// never Change settings, a role or a route need.
func TestNewPeopleRefusesWhatAMemberIsNotGiven(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	for _, list := range []string{`["ask","settings"]`, `["owner"]`, `["admin"]`, `["users"]`, `["signed_in"]`, `["nope"]`} {
		if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"new_people_perms":`+list+`}`); resp.StatusCode != 400 {
			t.Fatalf("%s: %d %s", list, resp.StatusCode, out)
		}
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/settings", ""); !strings.Contains(out, `"new_people_perms":["ask"]`) {
		t.Fatalf("a refused list was kept: %s", out)
	}
	// An empty list is allowed: new people then wait for the owner.
	setNewPeople(t, f, own, `[]`)
	if _, out := own.do(t, f, "GET", "/api/v1/auth/settings", ""); !strings.Contains(out, `"new_people_perms":[]`) {
		t.Fatalf("empty list: %s", out)
	}
	// An admin may not change it.
	adm := join(t, f, invite(t, f, own, `{"role":"admin"}`), "adm")
	if resp, _ := adm.do(t, f, "PUT", "/api/v1/auth/settings", `{"new_people_perms":["ask","auto_approve"]}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin changed it: %d", resp.StatusCode)
	}
}

// Every way a new member is made starts them with the owner's choice; an
// invite that names its own permissions keeps them.
func TestNewPeopleChoiceOnEveryWayIn(t *testing.T) {
	var logouts atomic.Int32
	ms := fakeMediaServer(t, &logouts)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	setNewPeople(t, f, own, `["ask","auto_approve","auto_approve_four_k"]`)
	const want = `["ask","auto_approve","auto_approve_four_k"]`

	// An invite with no permissions named.
	join(t, f, invite(t, f, own, `{"role":"member"}`), "bo")
	if got := permsOf(t, f, "bo"); got != want {
		t.Fatalf("invite: %s", got)
	}
	// An invite that names its own keeps them.
	join(t, f, invite(t, f, own, `{"role":"member","perms":["ask"]}`), "cy")
	if got := permsOf(t, f, "cy"); got != `["ask"]` {
		t.Fatalf("named invite: %s", got)
	}
	// An admin invite still starts with an admin's permissions.
	join(t, f, invite(t, f, own, `{"role":"admin"}`), "di")
	if got := permsOf(t, f, "di"); got != `["ask","auto_approve","approve","manage_library"]` {
		t.Fatalf("admin invite: %s", got)
	}
	// A first sign-in with Jellyfin.
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"`+ms.URL+`","join":"everyone"},"confirm_everyone":true}`)
	if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/media-server/jellyfin", `{"name":"mum","password":"right"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("jellyfin: %d %s", resp.StatusCode, out)
	}
	if got := permsOf(t, f, "Mum"); got != want {
		t.Fatalf("jellyfin: %s", got)
	}
	// Single sign-on.
	_, name, err := f.a.ssoIdentity(context.Background(), "oidc", "https://id.example.test", "sub-1", "eve@example.test", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := permsOf(t, f, name); got != want {
		t.Fatalf("single sign-on: %s", got)
	}
}

// A Kid never gets more than asking, whatever new people start with.
func TestNewPeopleKidKeepsToAsking(t *testing.T) {
	var logouts atomic.Int32
	ms := fakeMediaServer(t, &logouts)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	setNewPeople(t, f, own, `["ask","auto_approve","approve","manage_library"]`)
	// A Kid invite with no permissions named.
	join(t, f, invite(t, f, own, `{"role":"member","max_rating":"PG"}`), "kiddo")
	if got := permsOf(t, f, "kiddo"); got != `["ask","auto_approve"]` {
		t.Fatalf("kid invite: %s", got)
	}
	// A Kid invite that names more is still refused.
	if resp, _ := own.do(t, f, "POST", "/api/v1/users/invites", `{"role":"member","max_rating":"PG","perms":["ask","approve"]}`); resp.StatusCode != 400 {
		t.Fatalf("kid invite with approve: %d", resp.StatusCode)
	}
	// Someone the media server limits joins as a Kid.
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"emby":{"on":true,"url":"`+ms.URL+`","join":"everyone"},"confirm_everyone":true}`)
	if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/media-server/emby", `{"name":"kid","password":"right"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("emby kid: %d %s", resp.StatusCode, out)
	}
	if got := permsOf(t, f, "Kid"); got != `["ask","auto_approve"]` {
		t.Fatalf("emby kid: %s", got)
	}
}

// People bring over from Plex start with the choice too; a Kid there keeps
// to asking.
func TestNewPeoplePlexImport(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	setNewPeople(t, f, own, `["ask","auto_approve","approve"]`)
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`)
	own.do(t, f, "POST", "/api/v1/auth/import/plex/preview", "{}")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/import/plex", `{"people":[{"id":"21"},{"id":"22"}]}`); resp.StatusCode != 200 {
		t.Fatalf("import: %d %s", resp.StatusCode, out)
	}
	if got := permsOf(t, f, "ana"); got != `["ask","auto_approve","approve"]` {
		t.Fatalf("ana: %s", got)
	}
	if got := permsOf(t, f, "Little"); got != `["ask","auto_approve"]` {
		t.Fatalf("kid: %s", got)
	}
}

// Changing the choice changes nobody who is already here.
func TestNewPeopleChoiceLeavesPeopleAsTheyAre(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	join(t, f, invite(t, f, own, `{"role":"admin"}`), "adm")
	setNewPeople(t, f, own, `["ask","auto_approve","manage_library"]`)
	if got := permsOf(t, f, "ana"); got != `["ask"]` {
		t.Fatalf("ana changed: %s", got)
	}
	if got := permsOf(t, f, "adm"); got != `["ask","auto_approve","approve","manage_library"]` {
		t.Fatalf("admin changed: %s", got)
	}
}
