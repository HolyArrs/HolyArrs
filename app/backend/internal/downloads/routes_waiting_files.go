package downloads

import (
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// waitingForFiles means every outstanding decision on this download only
// needs a file to arrive. A manual decision in a mixed pack keeps Review.
func waitingForFiles(d domain.Download, slots map[int64]slotInfo) bool {
	n := 0
	for _, id := range d.SlotIDs {
		si, ok := slots[id]
		if !ok || mediafiles.SlotState(si.line.State) != mediafiles.SlotNeedsADecision || !si.active.Valid || si.active.Int64 != d.ID {
			continue
		}
		n++
		if !importer.FilesPending(si.lastReason) || importer.FolderMissing(si.lastReason) {
			return false
		}
	}
	return n > 0
}
