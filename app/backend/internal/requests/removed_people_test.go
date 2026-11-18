package requests

import (
	"context"
	"database/sql"
	"testing"
)

// A deleted person is not told about an ask and is not followed to it.
func TestDeletedPeopleAreNotTold(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Ask(ctx, r.bo, "693134"); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET removed_at = 'x' WHERE id = ?", r.bo.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ps, err := r.svc.people(ctx, res.Ask.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.ID == r.bo.UserID {
			t.Fatalf("a deleted person is still told: %+v", ps)
		}
	}
}
