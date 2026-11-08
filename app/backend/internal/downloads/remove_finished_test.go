package downloads

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestRemoveRefusesAFinishedDownload(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("remove finished: %v", err)
	}
	if err := m.MarkFailed(context.Background(), id, luis, "luis"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("mark failed finished: %v", err)
	}
	if len(w.fake.deleted) != 0 || w.download(t, id).State != "finished" {
		t.Fatalf("deleted %v", w.fake.deleted)
	}
}

func TestFinishedFromStalledFreesTheSlots(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	w.fake.set(hash, map[string]any{"name": "x", "category": "holyarrs", "progress": 0.2, "state": "stalledDL"})
	_, _ = m.SyncOnce(ctx)
	w.clock = w.clock.Add(11 * time.Minute)
	_, _ = m.SyncOnce(ctx)
	if sl := w.slot(t); sl.State != mediafiles.SlotStalled {
		t.Fatalf("slot %s", sl.State)
	}
	w.fake.set(hash, map[string]any{"progress": 1.0, "state": "stalledUP"})
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "finished" {
		t.Fatalf("download %s", d.State)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotDownloading {
		t.Fatalf("slot %s after finishing from stalled", sl.State)
	}
	if len(w.finshed) != 1 {
		t.Fatalf("OnFinished %v", w.finshed)
	}
}
