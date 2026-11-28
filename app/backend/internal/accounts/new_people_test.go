package accounts

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func permsOf(t *testing.T, f *fixture, id int64) string {
	t.Helper()
	var perms string
	if err := f.s.Read().QueryRow("SELECT perms FROM users WHERE id = ?", id).Scan(&perms); err != nil {
		t.Fatal(err)
	}
	return perms
}

type newPeopleView struct {
	Perms   []string `json:"perms"`
	Options []string `json:"options"`
}

func newPeopleOf(t *testing.T, f *fixture, p person) newPeopleView {
	t.Helper()
	resp, out := p.do(t, f, "GET", "/api/v1/users", "")
	if resp.StatusCode != 200 {
		t.Fatalf("users: %d %s", resp.StatusCode, out)
	}
	var v struct {
		NewPeople newPeopleView `json:"new_people"`
	}
	jsonOf(t, out, &v)
	return v.NewPeople
}

func startWith(t *testing.T, f *fixture, own person, list string) {
	t.Helper()
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/new-people", `{"perms":`+list+`}`); resp.StatusCode != 200 {
		t.Fatalf("set new-people list %s: %d %s", list, resp.StatusCode, out)
	}
}

const autoList = `["ask","auto_approve","auto_approve_four_k"]`

// route: GET /api/v1/users
// route: PUT /api/v1/users/new-people
// The list starts as today's behaviour (ask only), the owner changes it, and
// only the three request permissions a member may hold are accepted.
func TestNewPeopleListAndValidation(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	got := newPeopleOf(t, f, own)
	if strings.Join(got.Perms, ",") != "ask" || strings.Join(got.Options, ",") != "ask,auto_approve,auto_approve_four_k" {
		t.Fatalf("default list %+v", got)
	}
	startWith(t, f, own, `["auto_approve_four_k","auto_approve","ask","ask"]`)
	if got := newPeopleOf(t, f, own); strings.Join(got.Perms, ",") != "ask,auto_approve,auto_approve_four_k" {
		t.Fatalf("saved list is stored in screen order without repeats: %+v", got)
	}
	startWith(t, f, own, `[]`)
	if got := newPeopleOf(t, f, own); len(got.Perms) != 0 {
		t.Fatalf("an empty list is allowed: %+v", got)
	}
	before := newPeopleOf(t, f, own)
	for name, body := range map[string]string{
		"admin power":             `{"perms":["ask","approve"]}`,
		"library power":           `{"perms":["manage_library"]}`,
		"settings power":          `{"perms":["settings"]}`,
		"unknown":                 `{"perms":["ask","fly"]}`,
		"4K without auto-approve": `{"perms":["ask","auto_approve_four_k"]}`,
		"no list":                 `{}`,
		"not json":                `nope`,
	} {
		if resp, out := own.do(t, f, "PUT", "/api/v1/users/new-people", body); resp.StatusCode != 400 {
			t.Errorf("%s: %d %s", name, resp.StatusCode, out)
		}
	}
	if after := newPeopleOf(t, f, own); strings.Join(after.Perms, ",") != strings.Join(before.Perms, ",") {
		t.Fatalf("a refused list changed the setting: %+v", after)
	}
}

// Only the owner changes the list; an admin may read it (the
// invite form starts from it) and a member may not.
func TestNewPeopleOnlyOwnerChanges(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	mem := join(t, f, invite(t, f, own, `{}`), "bob")
	if resp, out := ad.do(t, f, "PUT", "/api/v1/users/new-people", `{"perms":["ask","auto_approve"]}`); resp.StatusCode != 403 {
		t.Fatalf("admin changed the list: %d %s", resp.StatusCode, out)
	}
	if resp, _ := mem.do(t, f, "PUT", "/api/v1/users/new-people", `{"perms":["ask","auto_approve"]}`); resp.StatusCode != 403 {
		t.Fatalf("member changed the list: %d", resp.StatusCode)
	}
	if resp, _ := mem.do(t, f, "GET", "/api/v1/users", ""); resp.StatusCode != 403 {
		t.Fatalf("member read the people list: %d", resp.StatusCode)
	}
	if got := newPeopleOf(t, f, ad); strings.Join(got.Perms, ",") != "ask" {
		t.Fatalf("admin reads %+v", got)
	}
	if permsOf(t, f, userID(t, f, "bob")) != `["ask"]` {
		t.Fatal("a refused change altered someone")
	}
	// The write sits behind the stale-session gate like other account changes.
	if !riskyRoutes["PUT /api/v1/users/new-people"] {
		t.Fatal("changing the list does not ask for a fresh sign-in")
	}
}

// People who sign in with a media server (outside link) or single sign-on
// start with the list; Kids stay ask-only; people already here keep theirs.
func TestNewPeopleStartWithTheList(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ctx := context.Background()
	join := func(ext string, kid bool) int64 {
		t.Helper()
		id, _, err := f.a.linkedUser(ctx, ProviderJellyfin, ext, ext, true, kid, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	before := join("before", false)
	if got := permsOf(t, f, before); got != `["ask"]` {
		t.Fatalf("default join perms %s", got)
	}

	startWith(t, f, own, `["ask","auto_approve"]`)
	if got := permsOf(t, f, join("after", false)); got != `["ask","auto_approve"]` {
		t.Fatalf("outside sign-in with the list set: %s", got)
	}
	if got := permsOf(t, f, before); got != `["ask"]` {
		t.Fatalf("the list changed someone already here: %s", got)
	}
	if got := permsOf(t, f, join("child", true)); got != `["ask"]` {
		t.Fatalf("kid joined with %s", got)
	}
	id, _, err := f.a.ssoIdentity(ctx, "oidc", "https://id.example.test", "sub-1", "sso@example.test", true, 0)
	if err != nil || permsOf(t, f, id) != `["ask","auto_approve"]` {
		t.Fatalf("single sign-on join: %v %s", err, permsOf(t, f, id))
	}

	startWith(t, f, own, autoList)
	if got := permsOf(t, f, join("four-k", false)); got != autoList {
		t.Fatalf("with 4K ticked: %s", got)
	}
	startWith(t, f, own, `[]`)
	if got := permsOf(t, f, join("none", false)); got != `[]` {
		t.Fatalf("with nothing ticked: %s", got)
	}
	if got := permsOf(t, f, join("none-kid", true)); got != `["ask"]` {
		t.Fatalf("kid with nothing ticked: %s", got)
	}
}

// A single sign-on person the groups make an admin keeps the admin defaults;
// the list is for members.
func TestNewPeopleListIsNotForAdmins(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	startWith(t, f, own, `["ask"]`)
	id, _, err := f.a.ssoIdentityRole(context.Background(), "oidc", "https://id.example.test", "sub-admin", "boss@example.test", true, webserver.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if got := permsOf(t, f, id); got != permsJSON(webserver.DefaultPerms(webserver.RoleAdmin)) {
		t.Fatalf("group admin: %s", got)
	}
}

// An invite that does not pick permissions gives what new people start with,
// limited to what the inviter holds; one that picks its own keeps them; an
// admin invite keeps the admin defaults.
func TestNewPeopleInvites(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	startWith(t, f, own, `["ask","auto_approve"]`)

	join(t, f, invite(t, f, own, `{}`), "plain")
	if got := permsOf(t, f, userID(t, f, "plain")); got != `["ask","auto_approve"]` {
		t.Fatalf("invite without perms: %s", got)
	}
	join(t, f, invite(t, f, own, `{"perms":["ask"]}`), "chosen")
	if got := permsOf(t, f, userID(t, f, "chosen")); got != `["ask"]` {
		t.Fatalf("invite that picked ask only: %s", got)
	}
	join(t, f, invite(t, f, own, `{"perms":[]}`), "bare")
	if got := permsOf(t, f, userID(t, f, "bare")); got != `[]` {
		t.Fatalf("invite that picked nothing: %s", got)
	}
	join(t, f, invite(t, f, own, `{"role":"admin"}`), "boss")
	if got := permsOf(t, f, userID(t, f, "boss")); got != permsJSON(webserver.DefaultPerms(webserver.RoleAdmin)) {
		t.Fatalf("admin invite: %s", got)
	}

	// An admin without auto-approve cannot hand it out, so the default they
	// make gives only what they hold (a request stays an invite, not a 403).
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ada")
	setSessionAssurance(t, f, "UPDATE users SET perms = ? WHERE name = 'ada'", `["ask","approve"]`)
	join(t, f, invite(t, f, ad, `{}`), "viaada")
	if got := permsOf(t, f, userID(t, f, "viaada")); got != `["ask"]` {
		t.Fatalf("admin without auto-approve made an invite with %s", got)
	}
	// The link keeps the list from the day it was made.
	tok := invite(t, f, own, `{}`)
	startWith(t, f, own, `["ask"]`)
	join(t, f, tok, "early")
	if got := permsOf(t, f, userID(t, f, "early")); got != `["ask","auto_approve"]` {
		t.Fatalf("invite made before the list changed: %s", got)
	}
}

// A Kid invite that picks no permissions asks only, whatever the list says.
func TestNewPeopleKidInviteAsksOnly(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	startWith(t, f, own, autoList)
	join(t, f, invite(t, f, own, `{"max_rating":"PG"}`), "kiddo")
	if got := permsOf(t, f, userID(t, f, "kiddo")); got != `["ask"]` {
		t.Fatalf("kid invite: %s", got)
	}
}

// Someone the owner imports from a media server starts with the list too,
// Kids ask-only.
func TestNewPeopleImportedFromMediaServer(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 200 {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	startWith(t, f, own, `["ask","auto_approve"]`)
	own.do(t, f, "POST", "/api/v1/auth/import/plex/preview", "{}")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/import/plex", `{"people":[{"id":"21"},{"id":"22"}]}`); resp.StatusCode != 200 {
		t.Fatalf("import: %d %s", resp.StatusCode, out)
	}
	if got := permsOf(t, f, userID(t, f, "ana")); got != `["ask","auto_approve"]` {
		t.Fatalf("imported member: %s", got)
	}
	if got := permsOf(t, f, userID(t, f, "Little")); got != `["ask"]` {
		t.Fatalf("imported kid: %s", got)
	}
}

// An install that turned on the earlier on/off switch keeps its behaviour:
// the old key reads as ask plus auto-approve until the owner saves a list.
func TestNewPeopleReadsTheOldSwitch(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, signInPrefix+"join_auto_approve", "true", domain.System, f.a.now())
	}); err != nil {
		t.Fatal(err)
	}
	if got := newPeopleOf(t, f, own); strings.Join(got.Perms, ",") != "ask,auto_approve" {
		t.Fatalf("old switch on: %+v", got)
	}
	id, _, err := f.a.linkedUser(context.Background(), ProviderJellyfin, "x", "x", true, false, nil)
	if err != nil || permsOf(t, f, id) != `["ask","auto_approve"]` {
		t.Fatalf("join under the old switch: %v %s", err, permsOf(t, f, id))
	}
	startWith(t, f, own, `["ask"]`)
	if got := newPeopleOf(t, f, own); strings.Join(got.Perms, ",") != "ask" {
		t.Fatalf("a saved list must win over the old switch: %+v", got)
	}
	// And the old switch off (or never set) reads as today's default.
	g := gated(t, Options{})
	if err := g.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, signInPrefix+"join_auto_approve", "false", domain.System, g.a.now())
	}); err != nil {
		t.Fatal(err)
	}
	if got := newPeopleOf(t, g, ownerOf(t, g)); strings.Join(got.Perms, ",") != "ask" {
		t.Fatalf("old switch off: %+v", got)
	}
}
