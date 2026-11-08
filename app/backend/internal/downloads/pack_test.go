package downloads

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestGrabPackFillsEverySlot(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	var others []int64
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var sl mediafiles.Slot
		if err := tx.QueryRow("SELECT item_id, version_id FROM slots WHERE id = ?", w.slotID).Scan(&sl.ItemID, &sl.VersionID); err != nil {
			return err
		}
		for _, st := range []mediafiles.SlotState{mediafiles.SlotChosen, mediafiles.SlotWanted} {
			it := &mediafiles.Item{TitleID: 1, Kind: "movie"}
			if err := tx.QueryRow("SELECT title_id FROM items WHERE id = ?", sl.ItemID).Scan(&it.TitleID); err != nil {
				return err
			}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			o := &mediafiles.Slot{ItemID: it.ID, VersionID: sl.VersionID, State: st}
			if st == mediafiles.SlotChosen {
				o.ChosenReleaseID = &w.relID
			}
			if _, err := database.Insert(tx, "slots", o, domain.System); err != nil {
				return err
			}
			others = append(others, o.ID)
		}
		ids := []int64{w.slotID, others[0], others[1]}
		_, err := tx.Exec(`UPDATE releases SET verdict = json_set(verdict, '$.pick_slot_ids', json(?)), matched_slot_ids = json(?) WHERE id = ?`,
			jsonIDs(ids), jsonIDs(ids), w.relID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	d := w.download(t, id)
	if !slices.Equal(d.SlotIDs, []int64{w.slotID, others[0]}) || len(w.fake.added) != 1 {
		t.Fatalf("download slots %v, added %d", d.SlotIDs, len(w.fake.added))
	}
	for _, sid := range []int64{w.slotID, others[0]} {
		var st string
		var dl sql.NullInt64
		_ = w.s.Read().QueryRow("SELECT state, active_download_id FROM slots WHERE id = ?", sid).Scan(&st, &dl)
		if st != string(mediafiles.SlotDownloading) || dl.Int64 != id {
			t.Fatalf("slot %d %s %v", sid, st, dl)
		}
	}
	var st string
	_ = w.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", others[1]).Scan(&st)
	if st != string(mediafiles.SlotWanted) {
		t.Fatalf("wanted slot moved to %s", st)
	}
}

func TestGrabPackLeavesSlotChosenForAnother(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	var other int64
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var sl mediafiles.Slot
		var titleID int64
		if err := tx.QueryRow("SELECT s.item_id, s.version_id, i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", w.slotID).Scan(&sl.ItemID, &sl.VersionID, &titleID); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: titleID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		var otherRel int64
		if err := tx.QueryRow(`INSERT INTO releases (created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol, size, state, verdict, matched_slot_ids, found_by)
			SELECT created_at, created_by, updated_at, updated_by, source_id, name || '.other', link_ref || 'x', protocol, size, state, verdict, matched_slot_ids, found_by FROM releases WHERE id = ? RETURNING id`, w.relID).Scan(&otherRel); err != nil {
			return err
		}
		o := &mediafiles.Slot{ItemID: it.ID, VersionID: sl.VersionID, State: mediafiles.SlotChosen, ChosenReleaseID: &otherRel}
		if _, err := database.Insert(tx, "slots", o, domain.System); err != nil {
			return err
		}
		other = o.ID
		ids := []int64{w.slotID, other}
		_, err := tx.Exec(`UPDATE releases SET verdict = json_set(verdict, '$.pick_slot_ids', json(?)) WHERE id = ?`, jsonIDs(ids), w.relID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); !slices.Equal(d.SlotIDs, []int64{w.slotID}) {
		t.Fatalf("download slots %v, want only %d", d.SlotIDs, w.slotID)
	}
	var st string
	_ = w.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", other).Scan(&st)
	if st != string(mediafiles.SlotChosen) {
		t.Fatalf("slot chosen for another Release moved to %s", st)
	}
}

func TestGrabPackKeepsToOneVersion(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	var other int64
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var itemID, titleID, lf, pr int64
		if err := tx.QueryRow("SELECT s.item_id, i.title_id, v.library_folder_id, v.pick_rules_id FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id WHERE s.id = ?",
			w.slotID).Scan(&itemID, &titleID, &lf, &pr); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: titleID, Label: "4K", PickRulesID: pr, LibraryFolderID: lf, FolderName: "Dune Part Two (2024) [4K]"}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		o := &mediafiles.Slot{ItemID: itemID, VersionID: v.ID, State: mediafiles.SlotChosen, ChosenReleaseID: &w.relID}
		if _, err := database.Insert(tx, "slots", o, domain.System); err != nil {
			return err
		}
		other = o.ID
		_, err := tx.Exec(`UPDATE releases SET verdict = json_set(verdict, '$.pick_slot_ids', json(?)) WHERE id = ?`, jsonIDs([]int64{w.slotID, other}), w.relID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); !slices.Equal(d.SlotIDs, []int64{w.slotID}) {
		t.Fatalf("download slots %v, want only the HD Slot %d", d.SlotIDs, w.slotID)
	}
	var st string
	_ = w.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", other).Scan(&st)
	if st != string(mediafiles.SlotChosen) {
		t.Fatalf("the 4K Slot moved to %s", st)
	}
}
