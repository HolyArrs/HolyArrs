package lists

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

func TestProbeSyncUsesDuePredicateWithoutWriting(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	l, err := Add(ctx, e.s, Input{Name: "Lab list", Kind: KindTMDB, Ref: "42", LibraryFolderID: e.lf.ID}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	writes := e.s.WriteCount()
	needed, _, err := e.svc.ProbeSync(ctx, now)
	if err != nil || !needed {
		t.Fatalf("new list: needed %v, error %v", needed, err)
	}
	if got := e.s.WriteCount(); got != writes {
		t.Fatalf("probe wrote %d transactions", got-writes)
	}
	// Set the same fields Sync persists, then the list is not due yet.
	if err := e.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE outside_lists SET last_synced_at = ? WHERE id = ?", store.FormatTime(now), l.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	writes = e.s.WriteCount()
	needed, _, err = e.svc.ProbeSync(ctx, now.Add(time.Hour))
	if err != nil || needed {
		t.Fatalf("fresh list: needed %v, error %v", needed, err)
	}
	if got := e.s.WriteCount(); got != writes {
		t.Fatalf("fresh probe wrote %d transactions", got-writes)
	}
}
