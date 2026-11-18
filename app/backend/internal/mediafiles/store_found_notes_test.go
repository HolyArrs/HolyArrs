package mediafiles

import (
	"context"
	"database/sql"
	"testing"
)

func TestFoundNotes(t *testing.T) {
	w := newCatalogWorld(t)
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET origin = 'found', state = 'present', leftover_reason = 'seasons_differ' WHERE id = ?", w.fileID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	notes, err := FoundNotes(ctx, w.s.Read(), w.v2)
	if err != nil || len(notes) != 1 || notes[0].ID != w.fileID || notes[0].Path == "" || notes[0].Reason != "seasons_differ" {
		t.Fatalf("notes %+v %v", notes, err)
	}
	if notes, err := FoundNotes(ctx, w.s.Read(), w.v1); err != nil || len(notes) != 0 {
		t.Fatalf("other version %+v %v", notes, err)
	}
}
