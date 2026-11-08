package accounts

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestSSOIdentityOwnerAndStableMapping(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	if _, _, err := f.a.ssoIdentity(ctx, "oidc", "https://id.example.test", "sub-1", "same@example.test", true, 0); err == nil {
		t.Fatal("external identity created the first owner")
	}
	setupOwner(t, f, client(t))
	id, name, err := f.a.ssoIdentity(ctx, "oidc", "https://id.example.test", "sub-1", "same@example.test", true, 0)
	if err != nil || id == 0 || name == "" {
		t.Fatal("member join failed")
	}
	again, _, err := f.a.ssoIdentity(ctx, "oidc", "https://id.example.test", "sub-1", "new@example.test", false, 0)
	if err != nil || again != id {
		t.Fatal("subject mapping changed with display name")
	}
	other, _, err := f.a.ssoIdentity(ctx, "oidc", "https://other.example.test", "sub-1", "same@example.test", true, 0)
	if err != nil || other == id {
		t.Fatal("different issuers merged identities")
	}
	second, _, err := f.a.ssoIdentity(ctx, "oidc", "https://id.example.test", "sub-2", "same@example.test", true, 0)
	if err != nil || second == id {
		t.Fatal("email matched a different subject")
	}
	var role, perms string
	if err := f.s.Read().QueryRow("SELECT role, perms FROM users WHERE id = ?", id).Scan(&role, &perms); err != nil || role != webserver.RoleMember || perms != `["ask"]` {
		t.Fatal("external identity elevated permissions")
	}
}

func TestSSOLinkRechecksLiveSessionInWriter(t *testing.T) {
	for _, tc := range []struct {
		name, statement string
		value           func(*fixture) string
	}{
		{"revoked", "DELETE FROM sessions", nil},
		{"disabled", "UPDATE users SET state = 'disabled'", nil},
		{"stale", "UPDATE sessions SET local_auth_at = ?", func(f *fixture) string { return database.FormatTime(f.clock.now().Add(-10 * time.Minute)) }},
		{"future", "UPDATE sessions SET local_auth_at = ?", func(f *fixture) string { return database.FormatTime(f.clock.now().Add(time.Second)) }},
		{"legacy", "UPDATE sessions SET local_auth_at = ''", nil},
		{"origin_changed", "UPDATE sessions SET auth_method = 'legacy'", nil},
		{"expired", "UPDATE sessions SET expires_at = ?", func(f *fixture) string { return database.FormatTime(f.clock.now()) }},
		{"family_changed", "UPDATE sessions SET family = 'replacement'", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, Options{})
			c := client(t)
			setupOwner(t, f, c)
			req := httptest.NewRequest("POST", "/api/v1/auth/oidc/link-finish", nil)
			for _, ck := range c.Jar.Cookies(mustURL(f.srv.URL)) {
				req.AddCookie(ck)
			}
			var id int64
			var family string
			if err := f.s.Read().QueryRow("SELECT user_id, family FROM sessions").Scan(&id, &family); err != nil {
				t.Fatal(err)
			}
			// The request and initiating family were captured while valid. A
			// revocation or policy change now wins before the insertion writer.
			if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
				var err error
				if tc.value == nil {
					_, err = tx.Exec(tc.statement)
				} else {
					_, err = tx.Exec(tc.statement, tc.value(f))
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.a.ssoLinkIdentity(req, "oidc", "https://id.example.test", "subject", "display", id, family); err == nil {
				t.Fatal("stale request linked an identity")
			}
			var count int
			if err := f.s.Read().QueryRow("SELECT count(*) FROM sso_links").Scan(&count); err != nil || count != 0 {
				t.Fatal("failed link left an identity")
			}
		})
	}
}

func TestSSOLinkAllowsOnlyOneIdentityPerUserAndIssuer(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	setupOwner(t, f, c)
	req := httptest.NewRequest("POST", "/api/v1/auth/forward/link", nil)
	for _, ck := range c.Jar.Cookies(mustURL(f.srv.URL)) {
		req.AddCookie(ck)
	}
	var id int64
	var family string
	if err := f.s.Read().QueryRow("SELECT user_id, family FROM sessions").Scan(&id, &family); err != nil {
		t.Fatal(err)
	}
	for n, subject := range []string{"first", "first", "different"} {
		_, _, err := f.a.ssoLinkIdentity(req, "oidc", "https://id.example.test", subject, "display", id, family)
		if (n < 2 && err != nil) || (n == 2 && err == nil) {
			t.Fatalf("identity %d uniqueness check failed: %v", n, err)
		}
	}
}
