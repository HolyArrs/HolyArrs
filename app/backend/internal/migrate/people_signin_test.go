package migrate

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// withPlexBea adds Bea, a Plex person of the old app with one request.
func withPlexBea(_, _, seerr *Snapshot) {
	seerr.Users = append(seerr.Users, User{SourceID: 3, Kind: "plex", ExternalID: "4242", ExtName: "beaplex", Name: "Bea", Permissions: []string{"REQUEST"}})
	seerr.Requests = append(seerr.Requests, Request{SourceID: 8, Kind: "movie", IDs: map[string]string{"tmdb": "605"}, State: "pending", By: 3, ByName: "Bea", Created: "2026-09-02T10:00:00Z"})
}

const beaKey = "person:seerr:plex:4242"

// beaRow reads Bea's account and where the Plex account 4242 signs in.
func beaRow(t *testing.T, l *moveLab) (id int64, state, hash, updated string, linkedTo int64) {
	t.Helper()
	if err := l.st.Read().QueryRow("SELECT id, state, password_hash, updated_at FROM users WHERE name = 'Bea'").Scan(&id, &state, &hash, &updated); err != nil {
		t.Fatal(err)
	}
	got, _, err := accounts.LinkedTo(context.Background(), l.st.Read(), "plex", "4242")
	if err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	return id, state, hash, updated, got
}

// The import links a Plex person to their Plex account; it turns no one on
// (their first Plex sign-in does, after its checks).
func TestPlexPersonComesOverLinked(t *testing.T) {
	l := newMoveLabWith(t, withPlexBea)
	p := l.plan(t)
	bea := item(t, p, GroupPeople, beaKey)
	if bea.Needs != "" || !strings.Contains(bea.Detail, "Plex account. Their first sign-in") || bea.Person.SignIn != "plex" || bea.Person.SignInID != "4242" {
		t.Fatalf("plan for a Plex person: %+v %+v", bea, bea.Person)
	}
	if ana := item(t, p, GroupPeople, "person:seerr:2"); ana.Needs != "an invite" || ana.Person.SignIn != "" {
		t.Fatalf("plan for a local person: %+v", ana)
	}
	dry := l.mv.Run(context.Background(), 0, p, true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, dry, beaKey); got.Result != ResCreate || got.Needs != "" || !strings.Contains(got.Detail, "first Plex sign-in") {
		t.Fatalf("dry run: %+v", got)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	id, state, hash, _, linked := beaRow(t, l)
	if state != "disabled" || hash != "" || linked != id {
		t.Fatalf("Bea: state %q, password %q, Plex signs in as %d (Bea is %d)", state, hash, linked, id)
	}
	var role, extName string
	if err := l.st.Read().QueryRow("SELECT u.role, l.external_name FROM users u JOIN user_links l ON l.user_id = u.id WHERE u.id = ?", id).Scan(&role, &extName); err != nil || role != "member" || extName != "beaplex" {
		t.Fatalf("role %q, Plex name %q: %v", role, extName, err)
	}
	if n := l.count(t, "SELECT count(*) FROM asks a JOIN ask_people p ON p.ask_id = a.id WHERE a.asked_by = ? AND p.user_id = ? AND a.tmdb_id = '605'", id, id); n != 1 {
		t.Fatal("Bea's request is not hers")
	}
	// A person who signed in to the old app with a password still waits for an invite.
	if n := l.count(t, "SELECT count(*) FROM users u WHERE name = 'Ana' AND state = 'disabled' AND password_hash = '' AND NOT EXISTS (SELECT 1 FROM user_links WHERE user_id = u.id)"); n != 1 {
		t.Fatal("Ana is not turned off without a link")
	}
	if got := lineOf(t, mv.Report, beaKey); got.Needs != "" || !hasNext(mv.Report, "People who signed in to the old app with Plex") {
		t.Fatalf("report: %+v %v", got, mv.Report.Next)
	}
	// Importing again changes nothing and makes no one twice.
	again := l.move(t, Choices{})
	if got := lineOf(t, again.Report, beaKey); got.Result != ResAlready {
		t.Fatalf("again: %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM users"); n != 3 {
		t.Fatalf("%d people after a second import, want owner, Ana and Bea", n)
	}
}

// People an earlier import brought over without their Plex sign-in get the
// link when the import runs again; their account and requests stay as they are.
func TestImportAgainGivesEarlierPeopleTheirSignIn(t *testing.T) {
	l := newMoveLabWith(t, withPlexBea)
	l.move(t, Choices{})
	id, _, _, _, _ := beaRow(t, l)
	// As an import before this left them.
	if err := l.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM user_links WHERE user_id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, _, _, before, _ := beaRow(t, l)
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, dry, beaKey); got.Result != ResChange {
		t.Fatalf("dry run: %+v", got)
	}
	if _, _, _, _, linked := beaRow(t, l); linked != 0 {
		t.Fatal("the dry run linked Bea")
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, beaKey); got.Result != ResChanged {
		t.Fatalf("again: %+v", got)
	}
	again, state, hash, after, linked := beaRow(t, l)
	if again != id || state != "disabled" || hash != "" || linked != id || after != before {
		t.Fatalf("Bea %d (was %d): %q, linked to %d, changed %s -> %s", again, id, state, linked, before, after)
	}
	if n := l.count(t, "SELECT count(*) FROM asks WHERE asked_by = ? AND tmdb_id = '605'", id); n != 1 {
		t.Fatal("Bea's request moved or doubled")
	}
	if n := l.count(t, "SELECT count(*) FROM users"); n != 3 {
		t.Fatalf("%d people", n)
	}
}

// A Plex account someone here already signs in with is that person: the
// import makes no second account and gives them the requests.
func TestPlexPersonAlreadyHereIsMatched(t *testing.T) {
	l := newMoveLabWith(t, withPlexBea)
	var mine int64
	if err := l.st.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO users (created_at, created_by, updated_at, updated_by, name, role, password_hash, state, perms)
			VALUES ('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', 'beaplex-2', 'member', '', 'active', '["ask"]')`)
		if err != nil {
			return err
		}
		mine, _ = res.LastInsertId()
		_, err = tx.Exec("INSERT INTO user_links (user_id, provider, external_id, external_name, created_at) VALUES (?, 'plex', '4242', 'beaplex', '2026-01-01T00:00:00Z')", mine)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, beaKey); got.Result != ResMatched {
		t.Fatalf("Bea: %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE name = 'Bea'"); n != 0 {
		t.Fatal("a second account for the same Plex person")
	}
	if n := l.count(t, "SELECT count(*) FROM asks WHERE asked_by = ? AND tmdb_id = '605'", mine); n != 1 {
		t.Fatal("the request is not with the account that signs in with Plex")
	}
}

// Taking the import back removes a linked person who never signed in.
func TestUndoRemovesUnusedLinkedPerson(t *testing.T) {
	l := newMoveLabWith(t, withPlexBea)
	mv := l.move(t, Choices{})
	if got := l.undo(t, mv.ID); got.State != "undone" {
		t.Fatalf("undo %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE name = 'Bea'"); n != 0 {
		t.Fatal("Bea stayed after the undo")
	}
	if n := l.count(t, "SELECT count(*) FROM user_links"); n != 0 {
		t.Fatal("her Plex link stayed")
	}
}
