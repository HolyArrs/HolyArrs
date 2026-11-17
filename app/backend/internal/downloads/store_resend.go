package downloads

import (
	"context"
	"database/sql"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// rowBusy holds for a Download row still in the Downloader's hands or waiting on
// a removal from it. A row that is not busy and that no Slot still follows is
// over, so it may start afresh when the same id comes back.
const rowBusy = `state IN (` + activeStates + `) OR removal_pending IS NOT NULL`

// sendSkip lists the ids on a Downloader that belong to other Download rows, so
// a send never takes them for this Release and Slot. A failed or vanished job
// must not come back by name, but a torrent is known by its hash: one whose row
// is over may be sent again, and that row then starts afresh.
func sendSkip(ctx context.Context, db database.Lister, downloaderID, releaseID, slotID int64, torrent bool) (map[string]bool, error) {
	followed := map[int64]bool{}
	if torrent {
		var err error
		if followed, err = mediafiles.FollowedDownloads(ctx, db); err != nil {
			return nil, err
		}
	}
	rows, err := db.QueryContext(ctx, `SELECT id, external_id, `+rowBusy+` FROM downloads WHERE downloader_id = ? AND NOT (release_id IS ?
		AND state IN (`+activeStates+`, 'finished') AND EXISTS (SELECT 1 FROM json_each(slot_ids) WHERE value = ?))`, downloaderID, releaseID, slotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	skip := map[string]bool{}
	for rows.Next() {
		var id int64
		var ext string
		var inUse bool
		if rows.Scan(&id, &ext, &inUse) != nil || torrent && !inUse && !followed[id] {
			continue
		}
		skip[ext] = true
	}
	return skip, rows.Err()
}

// heldBy finds the Download row that already holds an id on a Downloader (0
// when none does) and whether it is still in use, so it may not start afresh.
func heldBy(ctx context.Context, tx *sql.Tx, downloaderID int64, externalID string) (id int64, inUse bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id, `+rowBusy+` FROM downloads WHERE downloader_id = ? AND external_id = ?`, downloaderID, externalID).Scan(&id, &inUse)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil || inUse {
		return id, inUse, err
	}
	followed, err := mediafiles.FollowedDownloads(ctx, tx)
	return id, followed[id], err
}
