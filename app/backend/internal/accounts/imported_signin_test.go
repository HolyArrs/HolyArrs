package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// imported makes a person the way an import from Seerr does: turned off, no
// password, linked to their outside account.
func imported(t *testing.T, f *fixture, name, role, provider, extID, extName string, perms ...webserver.Perm) int64 {
	t.Helper()
	var id int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var err error
		u := domain.User{Name: name, Role: role, State: "disabled", Perms: permsJSON(perms)}
		if id, err = database.Insert(tx, "users", &u, domain.System); err != nil {
			return err
		}
		how, _, err := LinkImported(context.Background(), tx, id, provider, extID, extName, f.clock.t)
		if err == nil && how != LinkAdded {
			t.Errorf("link: %s", how)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func stateOf(t *testing.T, f *fixture, id int64) (state, role, perms, rating string) {
	t.Helper()
	if err := f.s.Read().QueryRow("SELECT state, role, perms, max_rating FROM users WHERE id = ?", id).Scan(&state, &role, &perms, &rating); err != nil {
		t.Fatal(err)
	}
	return
}

// route: POST /api/v1/auth/plex/finish
// route: POST /api/v1/auth/login
func TestImportedPlexPersonSignsInToTheirAccount(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	bea := imported(t, f, "Bea", webserver.RoleMember, ProviderPlex, "4242", "kidplex", webserver.PermAsk, webserver.PermAutoApprove)
	// Nobody new may join, but Bea is in already: her first sign-in turns her on.
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"nobody"}}`)
	c, code, out := plexSignIn(t, f, p)
	if code != 200 {
		t.Fatalf("plex sign-in: %d %s", code, out)
	}
	if _, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); !strings.Contains(me, `"name":"Bea"`) || !strings.Contains(me, `"role":"member"`) {
		t.Fatalf("me: %s", me)
	}
	if state, _, perms, rating := stateOf(t, f, bea); state != "active" || perms != permsJSON([]webserver.Perm{webserver.PermAsk, webserver.PermAutoApprove}) || rating != "" {
		t.Fatalf("after the first sign-in: %s %s %q", state, perms, rating)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users").Scan(&n)
	if n != 2 {
		t.Fatalf("%d people, want the owner and Bea", n)
	}
	// No password came over, and none opens the account.
	for _, typed := range []string{"", " ", "kidplex", "4242", pw} {
		c := client(t)
		body, _ := json.Marshal(map[string]string{"name": "Bea", "password": typed})
		resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", string(body), nil)
		if resp.StatusCode == 200 {
			t.Fatalf("password %q signed in: %s", typed, out)
		}
		if resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); resp.StatusCode == 200 {
			t.Fatalf("password %q left a session", typed)
		}
	}
}

// An imported person the owner turned off stays off: their Plex sign-in is
// refused as turned off, and it does not turn them on.
func TestImportedPersonTurnedOffStaysOff(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL})
	own := ownerOf(t, f)
	bea := imported(t, f, "Bea", webserver.RoleMember, ProviderPlex, "4242", "kidplex", webserver.PermAsk)
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/"+itoa(bea), `{"state":"disabled"}`); resp.StatusCode != 200 {
		t.Fatalf("turn off: %d %s", resp.StatusCode, out)
	}
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"everyone"},"confirm_everyone":true}`)
	if _, code, out := plexSignIn(t, f, p); code != 403 || !strings.Contains(out, "turned off") {
		t.Fatalf("turned off by the owner: %d %s", code, out)
	}
	if state, _, _, _ := stateOf(t, f, bea); state != "disabled" {
		t.Fatalf("state %s", state)
	}
}

// An imported person who no longer shares the owner's Plex server is refused
// and stays off, whatever the old app knew about them.
func TestImportedPersonOffTheServerIsRefused(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL})
	own := ownerOf(t, f)
	bea := imported(t, f, "Bea", webserver.RoleMember, ProviderPlex, "4242", "kidplex", webserver.PermAsk)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"other-server","join":"everyone"},"confirm_everyone":true}`)
	if _, code, out := plexSignIn(t, f, p); code != 403 || !strings.Contains(out, "not allowed") {
		t.Fatalf("off the server: %d %s", code, out)
	}
	if state, _, _, _ := stateOf(t, f, bea); state != "disabled" {
		t.Fatalf("state %s", state)
	}
	// With Plex sign-in off, nothing gets in.
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":false}}`)
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/plex/start", "", nil); resp.StatusCode != 404 {
		t.Fatalf("plex off: %d", resp.StatusCode)
	}
}

// A restricted Plex Home person lands on their imported account with the
// Kid limits a restricted person gets by the normal route, whatever role
// the import gave.
func TestImportedRestrictedPlexPersonGetsKidLimits(t *testing.T) {
	p := newFakePlex(t)
	p.restricted.Store(true)
	f := gated(t, Options{PlexURL: p.srv.URL})
	own := ownerOf(t, f)
	bea := imported(t, f, "Bea", webserver.RoleAdmin, ProviderPlex, "4242", "kidplex", webserver.PermAsk, webserver.PermApprove)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"nobody"}}`)
	c, code, out := plexSignIn(t, f, p)
	if code != 200 {
		t.Fatalf("plex sign-in: %d %s", code, out)
	}
	if _, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); !strings.Contains(me, `"name":"Bea"`) || !strings.Contains(me, `"max_rating":"PG"`) || !strings.Contains(me, `"perms":["ask"]`) {
		t.Fatalf("me: %s", me)
	}
	if state, role, _, _ := stateOf(t, f, bea); state != "active" || role != webserver.RoleMember {
		t.Fatalf("after: %s %s", state, role)
	}
}

// Jellyfin works the same way: signing in through the owner's server proves
// they are on it, and a person with parental limits there gets Kid limits.
func TestImportedJellyfinPeopleSignIn(t *testing.T) {
	var logouts atomic.Int32
	ms := fakeMediaServer(t, &logouts)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	mum := imported(t, f, "Mum", webserver.RoleMember, ProviderJellyfin, "abc123", "Mum", webserver.PermAsk, webserver.PermAutoApprove)
	kid := imported(t, f, "Kiddo", webserver.RoleMember, ProviderJellyfin, "kid1", "Kid", webserver.PermAsk, webserver.PermAutoApprove)
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"`+ms.URL+`","join":"nobody"}}`)
	for _, name := range []string{"mum", "kid"} {
		if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/media-server/jellyfin", `{"name":"`+name+`","password":"right"}`, nil); resp.StatusCode != 200 {
			t.Fatalf("%s: %d %s", name, resp.StatusCode, out)
		}
	}
	if state, _, perms, rating := stateOf(t, f, mum); state != "active" || perms != permsJSON([]webserver.Perm{webserver.PermAsk, webserver.PermAutoApprove}) || rating != "" {
		t.Fatalf("mum: %s %s %q", state, perms, rating)
	}
	if state, _, perms, rating := stateOf(t, f, kid); state != "active" || perms != permsJSON([]webserver.Perm{webserver.PermAsk}) || rating != "PG" {
		t.Fatalf("kid: %s %s %q", state, perms, rating)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users").Scan(&n)
	if n != 3 {
		t.Fatalf("%d people", n)
	}
}

// Settings > People > import from Plex skips an imported person.
func TestPlexPeopleImportSkipsImportedPerson(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	imported(t, f, "ana", webserver.RoleMember, ProviderPlex, "21", "ana", webserver.PermAsk)
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 200 {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/import/plex/preview", "{}"); resp.StatusCode != 200 {
		t.Fatalf("preview: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "POST", "/api/v1/auth/import/plex", `{"people":[{"id":"21"}]}`); !strings.Contains(out, `"skipped":1`) {
		t.Fatalf("import: %s", out)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users WHERE name LIKE 'ana%'").Scan(&n)
	if n != 1 {
		t.Fatalf("%d accounts for ana", n)
	}
}

// A second link for an outside account, or a second one of a kind for one
// person, is refused.
func TestLinkImportedKeepsOneLink(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	ana := imported(t, f, "ana", webserver.RoleMember, ProviderPlex, "21", "ana")
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		u := domain.User{Name: "bo", Role: webserver.RoleMember, State: "disabled", Perms: "[]"}
		bo, err := database.Insert(tx, "users", &u, domain.System)
		if err != nil {
			return err
		}
		if how, who, err := LinkImported(context.Background(), tx, bo, ProviderPlex, "21", "", f.clock.t); err != nil || how != LinkTaken || who != ana {
			t.Errorf("taken: %s %d %v", how, who, err)
		}
		if how, _, err := LinkImported(context.Background(), tx, ana, ProviderPlex, "99", "", f.clock.t); err != nil || how != LinkTaken {
			t.Errorf("second Plex for ana: %s %v", how, err)
		}
		if how, _, err := LinkImported(context.Background(), tx, ana, ProviderPlex, "21", "", f.clock.t); err != nil || how != LinkHad {
			t.Errorf("again: %s %v", how, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
