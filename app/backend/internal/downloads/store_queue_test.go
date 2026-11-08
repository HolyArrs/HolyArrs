package downloads

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The queue lists Downloads in flight, and finished or seeding ones only while
// a Slot still holds them.
func TestQueueRowsFilter(t *testing.T) {
	ctx := context.Background()
	w, _ := setup(t)
	slot := w.slot(t)
	ids := map[string]int64{}
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		add := func(name, state string, did int64, hold *mediafiles.Slot) error {
			dl := &domain.Download{DownloaderID: did, SlotIDs: []int64{}, ExternalID: name, State: state, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
			if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
				return err
			}
			ids[name] = dl.ID
			if hold != nil {
				_, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", dl.ID, hold.ID)
				return err
			}
			return nil
		}
		other := &mediafiles.Item{Kind: "movie"}
		if err := tx.QueryRow("SELECT title_id FROM items WHERE id = ?", slot.ItemID).Scan(&other.TitleID); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "items", other, domain.System); err != nil {
			return err
		}
		sl2 := &mediafiles.Slot{ItemID: other.ID, VersionID: slot.VersionID, State: mediafiles.SlotChosen}
		if _, err := database.Insert(tx, "slots", sl2, domain.System); err != nil {
			return err
		}
		for _, x := range []struct {
			name, state string
			did         int64
			hold        *mediafiles.Slot
		}{
			{"a", "downloading", 1, nil},
			{"b", "finished", 1, &slot},
			{"c", "finished", 1, nil},
			{"d", "failed", 1, sl2},
			{"e", "queued", 1, nil},
		} {
			if err := add(x.name, x.state, x.did, x.hold); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := QueueRows(ctx, w.s.Read())
	if err != nil {
		t.Fatal(err)
	}
	var got []int64
	for _, r := range rows {
		got = append(got, r.ID)
	}
	if want := []int64{ids["a"], ids["b"], ids["e"]}; !slices.Equal(got, want) {
		t.Fatalf("ids %v want %v", got, want)
	}
	if r := rows[2]; r.PathMap != "[]" || r.State != "queued" {
		t.Fatalf("last row: %+v", r)
	}
	if rows[0].DownloaderName != "Lab qBittorrent" || rows[0].Protocol != "torrent" {
		t.Fatalf("with downloader: %+v", rows[0])
	}
}
