package accounts

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// route: DELETE /api/v1/users/{id}
func TestDeletePersonEndsTheirWayIn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	pat := join(t, f, invite(t, f, own, `{}`), "pat")
	if resp, out := pat.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"k","scopes":["read"]}`); resp.StatusCode != 201 && resp.StatusCode != 403 {
		t.Fatalf("key: %d %s", resp.StatusCode, out)
	}
	id := userID(t, f, "pat")
	resp, out := own.do(t, f, "DELETE", fmt.Sprintf("/api/v1/users/%d", id), "")
	if resp.StatusCode != 204 {
		t.Fatalf("delete: %d %s", resp.StatusCode, out)
	}
	if resp, _ := pat.do(t, f, "GET", "/api/v1/titles", ""); resp.StatusCode != 401 {
		t.Fatalf("deleted person still signed in: %d", resp.StatusCode)
	}
	// The row stays so their asks and history keep pointing at someone.
	var name, state, hash, removed string
	if err := f.s.Read().QueryRow("SELECT name, state, password_hash, removed_at FROM users WHERE id = ?", id).
		Scan(&name, &state, &hash, &removed); err != nil {
		t.Fatal(err)
	}
	// They keep their real name, marked deleted, so it never matches a typed one.
	if name != fmt.Sprintf("pat\u2063%d", id) || state != "disabled" || hash != "" || removed == "" {
		t.Fatalf("row after delete: %q %q %q %q", name, state, hash, removed)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sessions WHERE user_id = ?", id).Scan(&n)
	var k int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM api_keys WHERE owner_user_id = ?", id).Scan(&k)
	if n != 0 || k != 0 {
		t.Fatalf("sessions %d keys %d left", n, k)
	}
	_, out = own.do(t, f, "GET", "/api/v1/users", "")
	if strings.Contains(out, "pat\u2063") || strings.Contains(out, `"name":"pat"`) {
		t.Fatalf("people list still shows them: %s", out)
	}
	// The name is free again.
	join(t, f, invite(t, f, own, `{}`), "pat")
	// A second delete finds no one.
	if resp, _ := own.do(t, f, "DELETE", fmt.Sprintf("/api/v1/users/%d", id), ""); resp.StatusCode != 404 {
		t.Fatalf("second delete: %d", resp.StatusCode)
	}
}

func TestDeletePersonLimits(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	ad2 := join(t, f, invite(t, f, own, `{"role":"admin"}`), "bob")
	_ = ad2
	join(t, f, invite(t, f, own, `{}`), "kid")
	ownerID, adminID, admin2ID, kidID := userID(t, f, "luis"), userID(t, f, "ana"), userID(t, f, "bob"), userID(t, f, "kid")
	for _, c := range []struct {
		who  person
		id   int64
		want int
		why  string
	}{
		{ad, ownerID, 403, "admin deletes the owner"},
		{own, ownerID, 403, "owner deletes themself"},
		{ad, adminID, 403, "admin deletes themself"},
		{ad, admin2ID, 403, "admin deletes another admin"},
		{ad, 999, 404, "no such person"},
		{ad, kidID, 204, "admin deletes a member"},
		{own, admin2ID, 204, "owner deletes an admin"},
	} {
		if resp, out := c.who.do(t, f, "DELETE", fmt.Sprintf("/api/v1/users/%d", c.id), ""); resp.StatusCode != c.want {
			t.Errorf("%s: %d %s", c.why, resp.StatusCode, out)
		}
	}
	kid := join(t, f, invite(t, f, own, `{}`), "kim")
	if resp, _ := kid.do(t, f, "DELETE", fmt.Sprintf("/api/v1/users/%d", adminID), ""); resp.StatusCode != 403 {
		t.Errorf("member deleted someone: %d", resp.StatusCode)
	}
}

// A deleted person stays gone: no route on /users/{id} finds them, so they
// cannot be turned back on, given a password or signed in as.
func TestDeletedPersonCannotComeBack(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{}`), "pat")
	id := userID(t, f, "pat")
	// Things only they had, which must go with them.
	for _, stmt := range []string{
		"INSERT INTO user_ask_settings(user_id) VALUES (?)",
		"INSERT INTO hidden(created_at, created_by, kind, tmdb_id, user_id) VALUES ('t', 's', 'movie', '1', ?)",
		"INSERT INTO user_views(user_id, name, value) VALUES (?, 'v', '{}')",
	} {
		if err := f.s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(stmt, id); return err }); err != nil {
			t.Fatal(err)
		}
	}
	path := fmt.Sprintf("/api/v1/users/%d", id)
	if resp, out := own.do(t, f, "DELETE", path, ""); resp.StatusCode != 204 {
		t.Fatalf("delete: %d %s", resp.StatusCode, out)
	}
	for _, c := range []struct{ method, path, body string }{
		{"PUT", path, `{"state":"active"}`},
		{"POST", path + "/password", `{"password":"` + pw + `"}`},
		{"POST", path + "/two-factor/reset", ""},
		{"POST", path + "/sessions/end-all", ""},
		{"GET", path + "/sessions", ""},
	} {
		if resp, out := own.do(t, f, c.method, c.path, c.body); resp.StatusCode != 404 {
			t.Errorf("%s %s after delete: %d %s", c.method, c.path, resp.StatusCode, out)
		}
	}
	// Even a direct write cannot turn the row back on.
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET state = 'active' WHERE id = ?", id)
		return err
	}); err == nil {
		t.Error("a deleted row was turned back on")
	}
	bare := client(t)
	for _, name := range []string{"pat", fmt.Sprintf("pat\u2063%d", id)} {
		if resp, _ := call(t, bare, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"`+name+`","password":"`+pw+`"}`, nil); resp.StatusCode == 200 {
			t.Error("signed in as a deleted person")
		}
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM user_ask_settings WHERE user_id = ?", id).Scan(&n)
	if n != 0 {
		t.Errorf("their request settings (email, Discord) stayed")
	}
	for _, q := range []string{"SELECT count(*) FROM hidden WHERE user_id = ?", "SELECT count(*) FROM user_views WHERE user_id = ?"} {
		var left int
		_ = f.s.Read().QueryRow(q, id).Scan(&left)
		if left != 0 {
			t.Errorf("%s: %d left", q, left)
		}
	}
}

// Asks a deleted person joined keep them, but never follow them again.
func TestDeletePersonStopsFollowingTheirAsks(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{}`), "pat")
	id := userID(t, f, "pat")
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, asked_by, asked_at, state)
			VALUES ('t', 's', 't', 's', 'movie', '1', 'x', ?, 't', 'asked')`, id)
		if err != nil {
			return err
		}
		ask, _ := res.LastInsertId()
		_, err = tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (?, ?, 't')", ask, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp, out := own.do(t, f, "DELETE", fmt.Sprintf("/api/v1/users/%d", id), ""); resp.StatusCode != 204 {
		t.Fatalf("delete: %d %s", resp.StatusCode, out)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM ask_people WHERE user_id = ? AND follow = 1", id).Scan(&n)
	var kept int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM ask_people WHERE user_id = ?", id).Scan(&kept)
	if n != 0 || kept != 1 {
		t.Fatalf("follow rows %d, kept %d", n, kept)
	}
}
