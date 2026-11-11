package shows

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The page's "X of Y episodes" leaves out specials unless they are kept up
// to date; their missing count still shows.
func TestInfoOfLeavesOutSpecials(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	ti := seedShow(t, s, labShow("tmdb", "100"), library.SeasonsAll)
	eps := episodes(t, s, ti.ID)
	set := func(q string, args ...any) {
		t.Helper()
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	set("UPDATE slots SET state = 'in_library' WHERE id IN (?, ?)", eps["S00E01"].SlotID, eps["S01E01"].SlotID)
	set("UPDATE parts SET keep_up_to_date = 0 WHERE title_id = ? AND number = 0", ti.ID)
	info, err := InfoOf(ctx, s.Read(), ti)
	if err != nil {
		t.Fatal(err)
	}
	if info.Episodes != 5 || info.InLibrary != 1 {
		t.Fatalf("specials not kept: %d of %d", info.InLibrary, info.Episodes)
	}
	set("UPDATE parts SET keep_up_to_date = 1 WHERE title_id = ? AND number = 0", ti.ID)
	if info, err = InfoOf(ctx, s.Read(), ti); err != nil || info.Episodes != 6 || info.InLibrary != 2 {
		t.Fatalf("specials kept: %+v %v", info, err)
	}
}
