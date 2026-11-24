package downloads

import (
	"database/sql"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A finished season pack whose missing episode was sent again in another
// download is settled for its own row: that episode waits on the other one.
func TestPackPhaseSkipsEpisodesHeldByAnotherDownload(t *testing.T) {
	t.Parallel()
	held := func(id int64, state string, active int64) slotInfo {
		si := slotInfo{line: EpisodeLine{SlotID: id, Season: 1, Episode: int(id), State: state}}
		if active != 0 {
			si.active = sql.NullInt64{Int64: active, Valid: true}
		}
		return si
	}
	const mine = 7
	for _, tc := range []struct {
		name  string
		slots []slotInfo
		want  string
	}{
		{"all placed", []slotInfo{held(1, "in_library", 0), held(2, "in_library", 0)}, "done"},
		{"one sent again elsewhere", []slotInfo{held(1, "in_library", 0), held(2, "downloading", 99)}, "done"},
		{"chosen for another download", []slotInfo{held(1, "in_library", 0), held(2, "chosen", 99)}, "done"},
		{"importing in another download", []slotInfo{held(1, "in_library", 0), held(2, "importing", 99)}, "done"},
		{"still mine to import", []slotInfo{held(1, "in_library", 0), held(2, "downloading", mine)}, "waiting_to_import"},
		{"mine, no active id yet", []slotInfo{held(1, "downloading", 0), held(2, "downloading", 0)}, "waiting_to_import"},
		{"mine importing", []slotInfo{held(1, "importing", mine), held(2, "downloading", 99)}, "importing"},
		{"needs a decision stays", []slotInfo{held(1, "needs_a_decision", mine), held(2, "downloading", 99)}, "needs_a_decision"},
	} {
		slots, ids := map[int64]slotInfo{}, []int64{}
		for _, si := range tc.slots {
			slots[si.line.SlotID] = si
			ids = append(ids, si.line.SlotID)
		}
		d := domain.Download{ID: mine, State: "finished", SlotIDs: ids}
		if got := phaseOfMany(d, linesOf(d.ID, ids, slots)); got != tc.want {
			t.Errorf("%s: phase %s, want %s", tc.name, got, tc.want)
		}
	}
}
