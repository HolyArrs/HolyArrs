package mediafiles

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// HandsOffDownloadedWhy is the reason a Slot carries once its download
// finished in hands-off mode. With no File it reads as "Downloaded".
const HandsOffDownloadedWhy = "Downloaded. Hands-off mode leaves the files in the Downloader."

// MarkDownloadedTx ends a Slot whose download finished in hands-off mode:
// it is in the Library with no File, good enough so nothing searches for it
// again, and let go of the download. A Slot that is not waiting for
// downloadID is left alone; the answer says whether it moved.
func MarkDownloadedTx(ctx context.Context, tx *sql.Tx, slotID, downloadID int64, by database.Actor) (bool, error) {
	var sl Slot
	if err := LoadSlot(ctx, tx, slotID, &sl); err != nil {
		return false, err
	}
	if sl.State != SlotDownloading && sl.State != SlotImporting {
		return false, nil
	}
	if sl.ActiveDownloadID != nil && *sl.ActiveDownloadID != downloadID {
		return false, nil
	}
	if sl.State == SlotDownloading {
		if err := MoveSlotTx(tx, slotID, SlotImporting, "", by); err != nil {
			return false, err
		}
	}
	if err := MoveSlotTx(tx, slotID, SlotInLibrary, HandsOffDownloadedWhy, by); err != nil {
		return false, err
	}
	_, err := tx.Exec("UPDATE slots SET active_download_id = NULL, good_enough = 1, updated_at = ? WHERE id = ?", database.FormatTime(time.Now()), slotID)
	return err == nil, err
}
