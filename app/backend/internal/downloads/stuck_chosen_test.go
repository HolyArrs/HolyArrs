package downloads

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// addSibling adds another Slot of the same title and Version, chosen for rel.
func addSibling(t *testing.T, w *world, rel int64) int64 {
	t.Helper()
	var id int64
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var sl mediafiles.Slot
		if err := database.Get(context.Background(), tx, "slots", w.slotID, &sl); err != nil {
			return err
		}
		var it mediafiles.Item
		if err := database.Get(context.Background(), tx, "items", sl.ItemID, &it); err != nil {
			return err
		}
		n := &mediafiles.Item{TitleID: it.TitleID, Kind: it.Kind}
		if _, err := database.Insert(tx, "items", n, domain.System); err != nil {
			return err
		}
		s2 := &mediafiles.Slot{ItemID: n.ID, VersionID: sl.VersionID, State: mediafiles.SlotChosen}
		if _, err := database.Insert(tx, "slots", s2, domain.System); err != nil {
			return err
		}
		id = s2.ID
		_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", rel, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// otherRelease adds a second Release of the same Source and returns its id.
func otherRelease(t *testing.T, w *world) int64 {
	t.Helper()
	var rel domain.Release
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		if err := database.Get(context.Background(), tx, "releases", w.relID, &rel); err != nil {
			return err
		}
		rel.ID, rel.Name, rel.InfoHash = 0, rel.Name+".PROPER", nil
		_, err := database.Insert(tx, "releases", &rel, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return rel.ID
}

func slotByID(t *testing.T, w *world, id int64) mediafiles.Slot {
	t.Helper()
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), w.s.Read(), "slots", id, &sl); err != nil {
		t.Fatal(err)
	}
	return sl
}

// A grab that gives up frees every Slot still chosen for the same Release (a
// season pack is one grab for many Slots), never a Slot chosen for another one.
func TestGiveUpFreesEverySlotOfThePack(t *testing.T) {
	cases := []struct {
		name      string
		otherRel  bool
		pending   bool
		wantState mediafiles.SlotState
	}{
		{"same release is freed", false, false, mediafiles.SlotWanted},
		{"other release stays", true, false, mediafiles.SlotChosen},
		{"a send still pending for it stays", false, true, mediafiles.SlotChosen},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, m := setup(t)
			rel := w.relID
			if c.otherRel {
				rel = otherRelease(t, w)
			}
			sib := addSibling(t, w, rel)
			if c.pending {
				if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
					var dr int64
					if err := tx.QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&dr); err != nil {
						return err
					}
					_, err := tx.Exec(`INSERT INTO downloader_attempts(downloader_id, release_id, slot_id, slot_snapshots, marker, category, state, created_at, updated_at)
						VALUES(?, ?, ?, json_array(json_object('id', ?)), 'm', 'c', 'pending', 'a', 'b')`, dr, rel, sib, sib)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			m.giveUp(context.Background(), w.slotID, w.relID, "the torrent belongs to another download", false, domain.System)
			if sl := w.slot(t); sl.State != mediafiles.SlotWanted {
				t.Fatalf("grabbed slot %s", sl.State)
			}
			if sl := slotByID(t, w, sib); sl.State != c.wantState {
				t.Fatalf("sibling %s, want %s", sl.State, c.wantState)
			}
		})
	}
}

// A chosen Slot still tied to an old Download that is over (removed, failed,
// vanished) is lost like any other: the sweep frees it. One tied to a Download
// still running for it stays.
func TestLostChoiceWithAnOldDownloadIsFreed(t *testing.T) {
	cases := []struct {
		state string
		want  mediafiles.SlotState
	}{
		{"cleaned", mediafiles.SlotWanted},
		{"failed", mediafiles.SlotWanted},
		{"vanished", mediafiles.SlotWanted},
		{"downloading", mediafiles.SlotChosen},
	}
	for _, c := range cases {
		t.Run(c.state, func(t *testing.T) {
			w, m := setup(t)
			ctx := context.Background()
			if err := w.s.Write(ctx, func(tx *sql.Tx) error {
				var dr int64
				if err := tx.QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&dr); err != nil {
					return err
				}
				now := database.FormatTime(time.Now())
				res, err := tx.Exec(`INSERT INTO downloads(downloader_id, external_id, release_id, slot_ids, state, removal_pending, started_by, created_at, created_by, updated_at, updated_by)
					VALUES(?, 'oldhash', NULL, '[]', ?, 1, 'system', ?, 'system', ?, 'system')`, dr, c.state, now, now)
				if err != nil {
					return err
				}
				id, _ := res.LastInsertId()
				_, err = tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", id, w.slotID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			m.Now = func() time.Time { return time.Now().Add(time.Hour) }
			m.settleLostChoices(ctx)
			if sl := w.slot(t); sl.State != c.want {
				t.Fatalf("slot %s, want %s", sl.State, c.want)
			}
		})
	}
}

// A removal that waits on a read only Downloader ends once the Downloader no
// longer has the torrent, so its hash is free to be sent again.
func TestPendingRemovalEndsWhenTheTorrentIsGone(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	if err := m.Remove(ctx, id, RemoveChoice{}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	m.retryRemovals(ctx)
	if p := removalPending(t, w, id); !p.Valid {
		t.Fatal("a read only Downloader that still has the torrent ended the removal")
	}
	w.fake.drop(hash)
	m.retryRemovals(ctx)
	if p := removalPending(t, w, id); p.Valid {
		t.Fatalf("removal still pending with the torrent gone: %+v", p)
	}
}
