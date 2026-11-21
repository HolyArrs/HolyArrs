package accounts

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestReadPersonAccess(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, perms, max_rating, removed_at) VALUES
			(1, '', '{}', '', '{}', 'luis', 'owner', '', '[]', '', ''),
			(2, '', '{}', '', '{}', 'kai', 'member', '', '["ask","youtube","nonsense"]', 'PG', ''),
			(3, '', '{}', '', '{}', 'gone', 'member', '', '["youtube"]', '', '2026-01-01T00:00:00Z')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	own, err := ReadPersonAccess(ctx, s.Read(), 1)
	if err != nil || own.Role != webserver.RoleOwner || !slices.Contains(own.Perms, webserver.PermYouTube) || own.Removed {
		t.Fatalf("owner: %+v %v", own, err)
	}
	kid, err := ReadPersonAccess(ctx, s.Read(), 2)
	if err != nil || kid.Name != "kai" || kid.MaxRating != "PG" || !slices.Equal(kid.Perms, []webserver.Perm{webserver.PermAsk, webserver.PermYouTube}) {
		t.Fatalf("kid: %+v %v", kid, err)
	}
	gone, err := ReadPersonAccess(ctx, s.Read(), 3)
	if err != nil || !gone.Removed {
		t.Fatalf("removed: %+v %v", gone, err)
	}
	if _, err := ReadPersonAccess(ctx, s.Read(), 99); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing person: %v", err)
	}
}
