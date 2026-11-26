package downloads

import (
	"database/sql"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestWaitingForFiles(t *testing.T) {
	d := domain.Download{ID: 7, SlotIDs: []int64{1, 2}}
	pending := func(reason string) slotInfo {
		return slotInfo{line: EpisodeLine{State: string(mediafiles.SlotNeedsADecision)}, active: sql.NullInt64{Int64: 7, Valid: true}, lastReason: reason}
	}
	cases := []struct {
		name  string
		slots map[int64]slotInfo
		want  bool
	}{
		{"all missing files", map[int64]slotInfo{1: pending(importer.CopyingWhy), 2: pending("the file for S02E08 could not be placed: the file is not in the download folder any more")}, true},
		{"a manual decision remains", map[int64]slotInfo{1: pending(importer.CopyingWhy), 2: pending("choose a file by hand")}, false},
		{"only another download owns the Slot", map[int64]slotInfo{1: {line: EpisodeLine{State: string(mediafiles.SlotNeedsADecision)}, active: sql.NullInt64{Int64: 8, Valid: true}, lastReason: importer.CopyingWhy}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := waitingForFiles(d, c.slots); got != c.want {
				t.Fatalf("waitingForFiles() = %v, want %v", got, c.want)
			}
		})
	}
}
