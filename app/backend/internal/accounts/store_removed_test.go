package accounts

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
)

// Someone turned off because their account was removed on the media server
// shows why on their card, and the reason goes when the owner turns them
// back on. The owner is never turned off.
func TestRemovedReasonShowsAndClears(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ctx := context.Background()
	var id int64
	err := f.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO users(created_at, created_by, updated_at, updated_by, name, role, password_hash, state)
			VALUES ('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', 'gone', 'member', '', 'active')`)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (?, 'emby', 'u7', 'gone', 't')`, id); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) SELECT id, 'emby', 'u9', name, 't' FROM users WHERE role = 'owner'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := TurnOffRemoved(ctx, f.s, "emby", map[string]bool{"u9": true}); err != nil {
		t.Fatal(err)
	}
	_, out := own.do(t, f, "GET", "/api/v1/users", "")
	if !strings.Contains(out, `"name":"gone","role":"member","state":"disabled"`) || !strings.Contains(out, `"off_reason":"emby"`) {
		t.Fatalf("after the check: %s", out)
	}
	if strings.Contains(out, `"role":"owner","state":"disabled"`) {
		t.Fatalf("the owner was turned off: %s", out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/"+strconv.FormatInt(id, 10), `{"state":"active"}`); resp.StatusCode != 200 {
		t.Fatalf("turn on: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/users", ""); strings.Contains(out, `"off_reason":"emby"`) {
		t.Fatalf("the reason stayed after turning on: %s", out)
	}
}

// linkedPeople adds the given members (name -> emby account id) and, when
// ownerExt is set, links the owner to that account.
func linkedPeople(t *testing.T, f *fixture, ownerExt string, people map[string]string) {
	t.Helper()
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for name, ext := range people {
			res, err := tx.Exec(`INSERT INTO users(created_at, created_by, updated_at, updated_by, name, role, password_hash, state)
				VALUES ('t', '{}', 't', '{}', ?, 'member', '', 'active')`, name)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			if _, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (?, 'emby', ?, ?, 't')`, id, ext, name); err != nil {
				return err
			}
		}
		if ownerExt == "" {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) SELECT id, 'emby', ?, name, 't' FROM users WHERE role = 'owner'`, ownerExt)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func personState(t *testing.T, f *fixture, name string) (state, why string) {
	t.Helper()
	if err := f.s.Read().QueryRow("SELECT state, off_reason FROM users WHERE name = ?", name).Scan(&state, &why); err != nil {
		t.Fatal(err)
	}
	return state, why
}

func turnOff(t *testing.T, f *fixture, present ...string) {
	t.Helper()
	p := map[string]bool{}
	for _, id := range present {
		p[id] = true
	}
	if err := TurnOffRemoved(context.Background(), f.s, "emby", p); err != nil {
		t.Fatal(err)
	}
}

func stopEvents(t *testing.T, f *fixture) int {
	t.Helper()
	var n int
	if err := f.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = 'health_broken' AND summary LIKE 'Not turning off%'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The only person linked on the server, really removed, is turned off: the
// more-than-half stop needs 3 or more linked people.
func TestRemovedOnlyLinkedPersonTurnsOff(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	linkedPeople(t, f, "", map[string]string{"solo": "u7"})
	turnOff(t, f, "u1")
	if s, _ := personState(t, f, "solo"); s != "disabled" {
		t.Fatalf("solo: %s", s)
	}
}

// One person removed per run, run after run: each run turns that one off,
// as people already off are not counted again.
func TestRemovedOneAtATime(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	linkedPeople(t, f, "u9", map[string]string{"a": "u1", "b": "u2", "c": "u3"})
	for i, run := range [][]string{{"u9", "u2", "u3"}, {"u9", "u3"}, {"u9"}} {
		turnOff(t, f, run...)
		name := []string{"a", "b", "c"}[i]
		if s, _ := personState(t, f, name); s != "disabled" {
			t.Fatalf("run %d: %s is %s", i+1, name, s)
		}
	}
	if stopEvents(t, f) != 0 {
		t.Fatal("the stop triggered")
	}
}

// A kept mark goes as soon as the account is back on the server, even when
// nobody is missing, so a later removal turns them off again.
func TestRemovedKeptMarkClearsWhenBack(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	linkedPeople(t, f, "u9", map[string]string{"a": "u1", "b": "u2", "c": "u3"})
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET off_reason = 'kept:emby' WHERE name = 'a'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	turnOff(t, f, "u9", "u1", "u2", "u3")
	if _, why := personState(t, f, "a"); why != "" {
		t.Fatalf("kept mark stayed: %q", why)
	}
	turnOff(t, f, "u9", "u2", "u3")
	if s, _ := personState(t, f, "a"); s != "disabled" {
		t.Fatalf("removed again but %s", s)
	}
}

// When more than half would go at once nothing changes, and one visible
// warning is written per stop, not every run.
func TestRemovedStopWarnsOnce(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	linkedPeople(t, f, "u9", map[string]string{"a": "u1", "b": "u2", "c": "u3", "d": "u4"})
	turnOff(t, f, "u9", "u4")
	turnOff(t, f, "u9", "u4")
	for _, n := range []string{"a", "b", "c", "d"} {
		if s, _ := personState(t, f, n); s != "active" {
			t.Fatalf("%s: %s", n, s)
		}
	}
	if n := stopEvents(t, f); n != 1 {
		t.Fatalf("warnings: %d", n)
	}
	turnOff(t, f, "u9", "u1", "u2", "u3", "u4")
	turnOff(t, f, "u9", "u4")
	if n := stopEvents(t, f); n != 2 {
		t.Fatalf("warnings after the stop cleared and came back: %d", n)
	}
}

func recoveredEvents(t *testing.T, f *fixture) int {
	t.Helper()
	var n int
	if err := f.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = 'health_recovered' AND group_key = 'removed_people:emby'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The owner's own link being gone does not hold back real removals.
func TestRemovedOwnerGoneStillTurnsOffOneAtATime(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	linkedPeople(t, f, "u9", map[string]string{"a": "u1", "b": "u2", "c": "u3"})
	for i, run := range [][]string{{"u9", "u2", "u3"}, {"u3"}, {"u8"}} {
		turnOff(t, f, run...)
		name := []string{"a", "b", "c"}[i]
		if s, _ := personState(t, f, name); s != "disabled" {
			t.Fatalf("run %d: %s is %s", i+1, name, s)
		}
	}
	if stopEvents(t, f) != 0 {
		t.Fatal("the stop triggered")
	}
}

// An empty list leaves the stop as it was; clearing it says so once.
func TestRemovedStopEmptyKeepsAndRecoversOnce(t *testing.T) {
	f := gated(t, Options{})
	ownerOf(t, f)
	linkedPeople(t, f, "u9", map[string]string{"a": "u1", "b": "u2", "c": "u3", "d": "u4"})
	turnOff(t, f, "u9", "u4")
	turnOff(t, f)
	turnOff(t, f, "u9", "u4")
	if n := stopEvents(t, f); n != 1 {
		t.Fatalf("warnings after an empty list: %d", n)
	}
	if n := recoveredEvents(t, f); n != 0 {
		t.Fatalf("recovered while still held back: %d", n)
	}
	turnOff(t, f, "u9", "u1", "u2", "u3", "u4")
	turnOff(t, f, "u9", "u1", "u2", "u3", "u4")
	if n := recoveredEvents(t, f); n != 1 {
		t.Fatalf("recovered events: %d", n)
	}
}

// With the sign-in way off, its people check is no longer reported.
func TestUnreadUserListsOnlyWhileSignInIsOn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ctx := context.Background()
	if err := NoteUserListRead(ctx, f.s, "jellyfin", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := UnreadUserLists(ctx, f.s.Read())["jellyfin"]; ok {
		t.Fatal("reported with jellyfin sign-in off")
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"jellyfin":{"on":true,"url":"http://127.0.0.1:1","join":"everyone"},"confirm_everyone":true}`); resp.StatusCode != 200 {
		t.Fatalf("way on: %d %s", resp.StatusCode, out)
	}
	if err := NoteUserListRead(ctx, f.s, "jellyfin", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := UnreadUserLists(ctx, f.s.Read())["jellyfin"]; !ok {
		t.Fatal("not reported with jellyfin sign-in on")
	}
}

// Turning the sign-in way off, or moving it to another address, drops the
// failed-read count at once: turning it on again shows no old warning.
func TestSignInChangeForgetsUnreadCount(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ctx := context.Background()
	put := func(body string) {
		t.Helper()
		if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", body); resp.StatusCode != 200 {
			t.Fatalf("%d %s", resp.StatusCode, out)
		}
	}
	on := func(url string) string {
		return `{"jellyfin":{"on":true,"url":"` + url + `","join":"everyone"},"confirm_everyone":true}`
	}
	fail3 := func() {
		for i := 0; i < 3; i++ {
			if err := NoteUserListRead(ctx, f.s, "jellyfin", false); err != nil {
				t.Fatal(err)
			}
		}
		if UnreadUserLists(ctx, f.s.Read())["jellyfin"] == "" {
			t.Fatal("no warning after 3 failed reads")
		}
	}
	put(on("http://127.0.0.1:1"))
	fail3()
	put(`{"jellyfin":{"on":false,"url":"http://127.0.0.1:1","join":"everyone"}}`)
	put(on("http://127.0.0.1:1"))
	if _, ok := UnreadUserLists(ctx, f.s.Read())["jellyfin"]; ok {
		t.Fatal("off and on again: the old count came back")
	}
	fail3()
	put(on("http://127.0.0.1:2"))
	if _, ok := UnreadUserLists(ctx, f.s.Read())["jellyfin"]; ok {
		t.Fatal("new address: the old count stayed")
	}
	fail3()
	put(on("http://127.0.0.1:2"))
	if UnreadUserLists(ctx, f.s.Read())["jellyfin"] == "" {
		t.Fatal("saving with nothing changed dropped the count")
	}
}
