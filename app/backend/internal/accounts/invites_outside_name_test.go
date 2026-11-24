package accounts

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// A key holder reads a deleted person on the invites list as "name (deleted)".
func TestInvitesNameDeletedPersonForKey(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', ?, 'admin', '')`, "maria⁣2"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (3, 't', '{}', 't', '{}', ?, 'member', '')`, "ana⁣3"); err != nil {
			return err
		}
		if err := insertInvite(ctx, tx, "h1", "2026-11-01T00:00:00Z", 2, "member", "[]", "", "2099-01-01T00:00:00Z", "", "", 1); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE invites SET used_at = 't', used_by = 3`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read := func(c context.Context) string {
		w := httptest.NewRecorder()
		f.a.listInvites(w, httptest.NewRequest(http.MethodGet, "/api/v1/users/invites", nil).WithContext(c))
		return w.Body.String()
	}
	if b := read(webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Role: webserver.RoleOwner, KeyID: 3})); strings.Contains(b, "⁣") || !strings.Contains(b, "maria (deleted)") || !strings.Contains(b, "ana (deleted)") {
		t.Fatalf("key read %s", b)
	}
	if b := read(webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Role: webserver.RoleOwner})); !strings.Contains(b, "maria⁣2") {
		t.Fatalf("app read %s", b)
	}
}
