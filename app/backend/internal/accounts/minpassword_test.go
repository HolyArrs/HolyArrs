package accounts

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// The shortest password is an owner setting: 8 by default, 4..64 allowed,
// and every place that takes a new password reads it.
func TestMinPasswordLengthSetting(t *testing.T) {
	f := gated(t, Options{})
	// Before the owner exists the state shows the default of 8.
	_, body := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if !strings.Contains(body, `"min_password_length":8`) {
		t.Fatalf("state: %s", body)
	}
	own := ownerOf(t, f)
	// By default a 7-character password is refused; the owner can lower the rule to 4.
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"`+pw+`","new":"sevench"}`); resp.StatusCode != 400 || !strings.Contains(out, "at least 8") {
		t.Fatalf("default short change: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"min_password_length":4}`); resp.StatusCode != 200 {
		t.Fatalf("set 4: %d %s", resp.StatusCode, out)
	}
	for _, bad := range []string{`3`, `65`, `"8"`} {
		if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"min_password_length":`+bad+`}`); resp.StatusCode != 400 {
			t.Fatalf("min %s taken: %d %s", bad, resp.StatusCode, out)
		}
	}
	// A short password works while the minimum is 4.
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"`+pw+`","new":"abcd"}`); resp.StatusCode != 204 {
		t.Fatalf("4-character password refused: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"abcd","new":"`+pw+`"}`); resp.StatusCode != 204 {
		t.Fatalf("change back: %d %s", resp.StatusCode, out)
	}
	resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"min_password_length":12}`)
	if resp.StatusCode != 200 || !strings.Contains(out, `"min_password_length":12`) {
		t.Fatalf("set 12: %d %s", resp.StatusCode, out)
	}
	if _, body := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil); !strings.Contains(body, `"min_password_length":12`) {
		t.Fatalf("state after: %s", body)
	}
	// Password change.
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"`+pw+`","new":"elevenchars"}`); resp.StatusCode != 400 || !strings.Contains(out, "at least 12") {
		t.Fatalf("short change: %d %s", resp.StatusCode, out)
	}
	// Invite accept.
	tok := invite(t, f, own, `{"role":"member"}`)
	p11 := pw[:11]
	resp, out = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"kid","password":"`+p11+`"}`, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("short invite password: %d %s", resp.StatusCode, out)
	}
	join(t, f, tok, "kid") // pw is long enough
	// reset-owner from the command line reads the same setting.
	if err := ResetOwner(context.Background(), f.s, "luis", "elevenchars"); err == nil {
		t.Fatal("reset-owner took a password under the minimum")
	}
	if err := ResetOwner(context.Background(), f.s, "luis", "twelve chars"); err != nil {
		t.Fatal(err)
	}
	if got := MinPasswordLength(context.Background(), f.s.Read()); got != 12 {
		t.Fatalf("stored: %d", got)
	}
}

func TestMinPasswordLengthClamps(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	for _, c := range []struct {
		v    string
		want int
	}{{"", DefaultMinPassword}, {"junk", DefaultMinPassword}, {"2", 4}, {"10", 10}, {"500", MaxMinPassword}} {
		if err := f.s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('this_app', 'sign_in_min_password_length', ?, '', '{}', '', '{}')
			ON CONFLICT(grp, key) DO UPDATE SET value = excluded.value`, c.v)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if got := MinPasswordLength(ctx, f.s.Read()); got != c.want {
			t.Errorf("%q: got %d want %d", c.v, got, c.want)
		}
	}
}
