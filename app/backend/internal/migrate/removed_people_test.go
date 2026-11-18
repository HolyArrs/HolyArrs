package migrate

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// An old-app person never merges into a deleted person's row, even when
// the names match.
func TestMoveSkipsDeletedPeople(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		id, err := database.Insert(tx, "users", &domain.User{Name: "ana", Role: "member", PasswordHash: "", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE users SET removed_at = 'x' WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{People: map[string]string{"person:seerr:2": "merge"}})
	if got := lineOf(t, mv.Report, "person:seerr:2"); got.Result == ResMatched || got.Choose == ChooseSameName {
		t.Fatalf("merged into a deleted row: %+v", got)
	}
}

// A person whose name only a deleted row holds comes over with a free name.
func TestMoveNamesAroundDeletedPeople(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		id, err := database.Insert(tx, "users", &domain.User{Name: "ana", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE users SET removed_at = 'x' WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "person:seerr:2"); got.Result != ResCreated {
		t.Fatalf("not brought over: %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE name = 'Ana (2)' AND removed_at = ''"); n != 1 {
		t.Fatal("no free name given")
	}
}
