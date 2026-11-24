package importer

import (
	"context"
	"database/sql"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// ErrStandInFolder refuses importing into a title that was added in
// hands-off mode with no Library folder, now that the mode is off.
var ErrStandInFolder = errors.New("this title has no Library folder, so its download cannot be imported. Choose a Library folder for it on its page, then import again")

// handsOff answers an import in hands-off mode: the files stay where the
// Downloader put them and each Slot that waited for d reads as downloaded.
// Nothing on disk is read or written. ok is false when the mode is off; a
// title that has only the stand-in folder then gets ErrStandInFolder.
func (im *Importer) handsOff(ctx context.Context, d domain.Download, dryRun bool, by domain.Actor) (imp *mediafiles.Import, ok bool, err error) {
	db := im.Store.Read()
	if !parts.HandsOff(ctx, db) {
		for _, sid := range d.SlotIDs {
			if mediafiles.SlotStandIn(ctx, db, sid) {
				return nil, true, ErrStandInFolder
			}
		}
		return nil, false, nil
	}
	imp = &mediafiles.Import{DownloadID: &d.ID, Plan: []mediafiles.PlanStep{}, Result: []mediafiles.PlaceResult{}, Mode: "dry_run", State: "planned"}
	if dryRun {
		return imp, true, nil
	}
	var moved []int64
	err = im.Store.Write(ctx, func(tx *sql.Tx) error {
		moved = moved[:0]
		for _, sid := range d.SlotIDs {
			done, err := mediafiles.MarkDownloadedTx(ctx, tx, sid, d.ID, by)
			if errors.Is(err, database.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if done {
				moved = append(moved, sid)
			}
		}
		return nil
	})
	if err != nil {
		return nil, true, err
	}
	for _, sid := range moved {
		im.publish("slot", map[string]any{"id": sid, "state": mediafiles.SlotInLibrary, "reason": mediafiles.HandsOffDownloadedWhy})
	}
	imp.Mode, imp.State = "hands_off", "done"
	return imp, true, nil
}
