package downloads

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestPackStallsAndFailsForEverySlot(t *testing.T) {
	w, m, id, ids := packWorld(t)
	ctx := context.Background()
	w.clock = w.clock.Add(11 * time.Minute)
	_, _ = m.SyncOnce(ctx)
	if got := slotStates(t, w, ids); !slices.Equal(got, []string{"stalled", "stalled", "stalled"}) {
		t.Fatalf("after the stall time %v", got)
	}
	w.clock = w.clock.Add(24 * time.Hour)
	_, _ = m.SyncOnce(ctx)
	if got := slotStates(t, w, ids); !slices.Equal(got, []string{"failed", "failed", "failed"}) {
		t.Fatalf("after the act time %v", got)
	}
	if r := w.release(t); r.State != "turned_down" {
		t.Fatalf("release %s", r.State)
	}
	if d := w.download(t, id); d.State != "failed" {
		t.Fatalf("download %s", d.State)
	}
	if !slices.Equal(w.failedSlots, ids) {
		t.Fatalf("searched again %v, want %v", w.failedSlots, ids)
	}
}

func TestMarkFailedOnAPack(t *testing.T) {
	w, m, id, ids := packWorld(t)
	if err := m.MarkFailed(context.Background(), id, domain.Actor{Kind: "user", ID: 1}, "luis"); err != nil {
		t.Fatal(err)
	}
	if got := slotStates(t, w, ids); !slices.Equal(got, []string{"failed", "failed", "failed"}) || !slices.Equal(w.failedSlots, ids) {
		t.Fatalf("slots %v, searched %v", got, w.failedSlots)
	}
	if r := w.release(t); r.State != "turned_down" {
		t.Fatalf("release %s", r.State)
	}
}

func packWorld(t *testing.T) (*world, *Manager, int64, []int64) {
	t.Helper()
	w, m := setup(t)
	ctx := context.Background()
	ids := []int64{w.slotID}
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var sl mediafiles.Slot
		if err := tx.QueryRow("SELECT item_id, version_id FROM slots WHERE id = ?", w.slotID).Scan(&sl.ItemID, &sl.VersionID); err != nil {
			return err
		}
		for i := 0; i < 2; i++ {
			it := &mediafiles.Item{Kind: "movie"}
			if err := tx.QueryRow("SELECT title_id FROM items WHERE id = ?", sl.ItemID).Scan(&it.TitleID); err != nil {
				return err
			}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			o := &mediafiles.Slot{ItemID: it.ID, VersionID: sl.VersionID, State: mediafiles.SlotChosen, ChosenReleaseID: &w.relID}
			if _, err := database.Insert(tx, "slots", o, domain.System); err != nil {
				return err
			}
			ids = append(ids, o.ID)
		}
		_, err := tx.Exec(`UPDATE releases SET verdict = json_set(verdict, '$.pick_slot_ids', json(?)), matched_slot_ids = json(?) WHERE id = ?`,
			jsonIDs(ids), jsonIDs(ids), w.relID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	id := grabbed(t, w, m)
	if d := w.download(t, id); !slices.Equal(d.SlotIDs, ids) {
		t.Fatalf("pack slots %v", d.SlotIDs)
	}
	return w, m, id, ids
}

func slotStates(t *testing.T, w *world, ids []int64) []string {
	t.Helper()
	var out []string
	for _, id := range ids {
		var st string
		_ = w.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", id).Scan(&st)
		out = append(out, st)
	}
	return out
}
