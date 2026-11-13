package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// imported makes a person the way an import from Seerr does: turned off, no
// password, then linked to their outside account, which turns them on.
func imported(t *testing.T, f *fixture, name, provider, extID, extName string) int64 {
	t.Helper()
	var id int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var err error
		u := domain.User{Name: name, Role: webserver.RoleMember, State: "disabled", Perms: permsJSON([]webserver.Perm{webserver.PermAsk})}
		if id, err = database.Insert(tx, "users", &u, domain.System); err != nil {
			return err
		}
		how, _, err := LinkImported(context.Background(), tx, id, provider, extID, extName, f.clock.t, domain.System)
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

// route: POST /api/v1/auth/plex/finish
// route: POST /api/v1/auth/login
func TestImportedPlexPersonSignsInToTheirAccount(t *testing.T) {
	p := newFakePlex(t)
	f := gated(t, Options{PlexURL: p.srv.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	bea := imported(t, f, "Bea", ProviderPlex, "4242", "kidplex")
	var state string
	if err := f.s.Read().QueryRow("SELECT state FROM users WHERE id = ?", bea).Scan(&state); err != nil || state != "active" {
		t.Fatalf("imported person: %q %v", state, err)
	}
	// Nobody new may join: Bea is in already.
	own.do(t, f, "PUT", "/api/v1/auth/settings", `{"plex":{"on":true,"server_id":"machine-1","join":"nobody"}}`)
	c, code, out := plexSignIn(t, f, p)
	if code != 200 {
		t.Fatalf("plex sign-in: %d %s", code, out)
	}
	if _, me := call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil); !strings.Contains(me, `"name":"Bea"`) || !strings.Contains(me, `"role":"member"`) {
		t.Fatalf("me: %s", me)
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
	// Turned off by the owner, the Plex sign-in stops too.
	own.do(t, f, "PUT", "/api/v1/users/"+itoa(bea), `{"state":"disabled"}`)
	if _, code, _ := plexSignIn(t, f, p); code != 403 {
		t.Fatalf("turned off: %d", code)
	}
}

// Settings > People > import from Plex skips an imported person.
func TestPlexPeopleImportSkipsImportedPerson(t *testing.T) {
	p := fakePlexOwner(t)
	f := gated(t, Options{PlexURL: p.URL, PlexAppURL: "https://app.example.test"})
	own := ownerOf(t, f)
	imported(t, f, "ana", ProviderPlex, "21", "ana")
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
// person, is refused; nobody is turned on by it.
func TestLinkImportedKeepsOneLink(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	ana := imported(t, f, "ana", ProviderPlex, "21", "ana")
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		u := domain.User{Name: "bo", Role: webserver.RoleMember, State: "disabled", Perms: "[]"}
		bo, err := database.Insert(tx, "users", &u, domain.System)
		if err != nil {
			return err
		}
		if how, who, err := LinkImported(context.Background(), tx, bo, ProviderPlex, "21", "", f.clock.t, domain.System); err != nil || how != LinkTaken || who != ana {
			t.Errorf("taken: %s %d %v", how, who, err)
		}
		if how, _, err := LinkImported(context.Background(), tx, ana, ProviderPlex, "99", "", f.clock.t, domain.System); err != nil || how != LinkTaken {
			t.Errorf("second Plex for ana: %s %v", how, err)
		}
		if how, _, err := LinkImported(context.Background(), tx, ana, ProviderPlex, "21", "", f.clock.t, domain.System); err != nil || how != LinkHad {
			t.Errorf("again: %s %v", how, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var off int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users WHERE name = 'bo' AND state = 'disabled'").Scan(&off)
	if off != 1 {
		t.Fatal("bo was turned on without a link")
	}
}
