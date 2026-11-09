package accounts

import (
	"context"
	"database/sql"
	"testing"
)

// A Plex or single sign-on identity linked by someone who knew the old
// password stops working when the password is changed, set by an admin or
// reset from the command line.
func TestPasswordChangeEndsOutsideLinks(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ctx := context.Background()
	link := func(name string) {
		t.Helper()
		if err := f.s.WriteCore(ctx, func(tx *sql.Tx) error {
			var id int64
			if err := tx.QueryRow("SELECT id FROM users WHERE name = ?", name).Scan(&id); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (?, 'plex', ?, 'x', '2026-01-01T00:00:00Z')", id, "plex-"+name); err != nil {
				return err
			}
			_, err := tx.Exec("INSERT INTO sso_links(kind, issuer, subject, user_id, display_name, created_at) VALUES ('oidc', 'https://idp.example', ?, ?, 'x', '2026-01-01T00:00:00Z')", "sub-"+name, id)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	links := func(name string) int {
		t.Helper()
		var n int
		if err := f.s.Read().QueryRow(`SELECT (SELECT count(*) FROM user_links l JOIN users u ON u.id = l.user_id WHERE u.name = ?)
			+ (SELECT count(*) FROM sso_links l JOIN users u ON u.id = l.user_id WHERE u.name = ?)`, name, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Own change.
	link("luis")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"`+pw+`","new":"another long passphrase"}`); resp.StatusCode != 204 {
		t.Fatalf("change: %d %s", resp.StatusCode, out)
	}
	if n := links("luis"); n != 0 {
		t.Fatalf("%d outside sign-ins kept after the owner's own change", n)
	}

	// Set by the owner for a member; other people's links stay.
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	_ = ana
	link("ana")
	link("luis")
	var anaID string
	_ = f.s.Read().QueryRow("SELECT id FROM users WHERE name = 'ana'").Scan(&anaID)
	if resp, out := own.do(t, f, "POST", "/api/v1/users/"+anaID+"/password", `{"password":"a fresh long passphrase"}`); resp.StatusCode/100 != 2 {
		t.Fatalf("set: %d %s", resp.StatusCode, out)
	}
	if n := links("ana"); n != 0 {
		t.Fatalf("%d outside sign-ins kept after an admin set the password", n)
	}
	if n := links("luis"); n != 2 {
		t.Fatalf("someone else's links went too: %d", n)
	}

	// reset-owner from the command line.
	if err := ResetOwner(ctx, f.s, "luis", "yet another long passphrase"); err != nil {
		t.Fatal(err)
	}
	if n := links("luis"); n != 0 {
		t.Fatalf("%d outside sign-ins kept after reset-owner", n)
	}
}
