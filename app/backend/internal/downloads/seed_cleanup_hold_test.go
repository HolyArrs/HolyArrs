package downloads

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A partly done import holds seeding cleanup while one of its
// episodes still waits, and lets it go once a later import fills it or the
// person stops wanting it.
func TestSeedCleanupPartlyDoneHoldsUntilNothingWaits(t *testing.T) {
	for _, release := range []string{
		`UPDATE slots SET state = 'in_library'`,
		`UPDATE items SET keep_up_to_date = 0`,
	} {
		w, m, id, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "stoppedUP"})
		if err := execSQL(w, `UPDATE imports SET state = 'partly_done'`); err != nil {
			t.Fatal(err)
		}
		if err := execSQL(w, `UPDATE slots SET state = 'wanted', active_download_id = NULL; UPDATE items SET keep_up_to_date = 1`); err != nil {
			t.Fatal(err)
		}
		m.cleanSeeded(context.Background())
		if len(eng.removed) != 0 {
			t.Fatalf("removed while an episode waits: %v", eng.removed)
		}
		wait, err := mediafiles.CleanupWaiting(context.Background(), m.Store.Read(), downloadSlots(t, m, id))
		if err != nil || len(wait[id]) != 1 {
			t.Fatalf("waiting %v %v", wait, err)
		}
		if err := execSQL(w, release); err != nil {
			t.Fatal(err)
		}
		m.cleanSeeded(context.Background())
		if len(eng.removed) != 1 {
			t.Fatalf("%s: not released: %v", release, eng.removed)
		}
		if wait, _ := mediafiles.CleanupWaiting(context.Background(), m.Store.Read(), downloadSlots(t, m, id)); len(wait) != 0 {
			t.Fatalf("%s: still waiting %v", release, wait)
		}
	}
}

// downloadSlots maps the download to its Slots, as the cleanup check reads them.
func downloadSlots(t *testing.T, m *Manager, id int64) map[int64][]int64 {
	t.Helper()
	var d domain.Download
	if err := database.Get(context.Background(), m.Store.Read(), "downloads", id, &d); err != nil {
		t.Fatal(err)
	}
	return map[int64][]int64{id: d.SlotIDs}
}

// A download whose newest import failed holds seeding cleanup while its
// episodes still wait, so the person can import it by hand, and lets it go
// once they are filled from elsewhere or no longer wanted.
func TestSeedCleanupFailedImportHoldsUntilNothingWaits(t *testing.T) {
	for _, release := range []string{
		`UPDATE slots SET state = 'in_library'`,
		`UPDATE slots SET state = 'removed'`,
	} {
		w, m, id, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "stoppedUP"})
		if err := execSQL(w, `UPDATE imports SET state = 'failed'`); err != nil {
			t.Fatal(err)
		}
		if err := execSQL(w, `UPDATE slots SET state = 'wanted', active_download_id = NULL; UPDATE items SET keep_up_to_date = 1`); err != nil {
			t.Fatal(err)
		}
		m.cleanSeeded(context.Background())
		if len(eng.removed) != 0 {
			t.Fatalf("removed while an episode waits: %v", eng.removed)
		}
		wait, err := mediafiles.CleanupWaiting(context.Background(), m.Store.Read(), downloadSlots(t, m, id))
		if err != nil || len(wait[id]) != 1 {
			t.Fatalf("hold not shown: %v %v", wait, err)
		}
		if err := execSQL(w, release); err != nil {
			t.Fatal(err)
		}
		m.cleanSeeded(context.Background())
		if len(eng.removed) != 1 {
			t.Fatalf("%s: not released: %v", release, eng.removed)
		}
		if wait, _ := mediafiles.CleanupWaiting(context.Background(), m.Store.Read(), downloadSlots(t, m, id)); len(wait) != 0 {
			t.Fatalf("%s: still waiting %v", release, wait)
		}
	}
}
