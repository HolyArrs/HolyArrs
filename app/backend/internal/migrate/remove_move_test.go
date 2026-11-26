package migrate

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// an undone import, or a failed one that made nothing, can be removed
// from Earlier imports; one that still holds what it made cannot.
func TestRemoveMove(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	mk := func(state string, made bool) int64 {
		id := newTestMove(t, l)
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec("UPDATE moves SET state = ? WHERE id = ?", state, id); err != nil {
				return err
			}
			if made {
				_, err := tx.Exec("INSERT INTO move_made (move_id, key, what, row_id, created_at) VALUES (?, 'k', 'source', 1, '2026-01-01')", id)
				return err
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	undone, failedEmpty, failedMade, done, running := mk("undone", true), mk("failed", false), mk("failed", true), mk("done", false), mk("running", false)
	for _, id := range []int64{undone, failedEmpty} {
		if err := RemoveMove(ctx, l.st, id); err != nil {
			t.Fatalf("remove %d: %v", id, err)
		}
		if _, err := GetMove(ctx, l.st, id); !errors.Is(err, ErrNoMove) {
			t.Fatalf("move %d still there: %v", id, err)
		}
	}
	for _, id := range []int64{failedMade, done, running} {
		if err := RemoveMove(ctx, l.st, id); !errors.Is(err, ErrMoveKept) {
			t.Fatalf("remove %d: %v, want ErrMoveKept", id, err)
		}
	}
	if err := RemoveMove(ctx, l.st, 999); !errors.Is(err, ErrNoMove) {
		t.Fatalf("missing: %v", err)
	}
}
