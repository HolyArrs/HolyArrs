package mediafiles

import (
	"context"
	"database/sql"
)

// SlotStillEmptyTx is true while the slot belongs to the version, holds no
// file and is not removed.
func SlotStillEmptyTx(ctx context.Context, tx *sql.Tx, slotID, versionID int64) (bool, error) {
	var free int
	err := tx.QueryRowContext(ctx, "SELECT count(*) FROM slots WHERE id = ? AND version_id = ? AND file_id IS NULL AND state != 'removed'", slotID, versionID).Scan(&free)
	return free > 0, err
}

// SetReleaseNameTx sets the release name a file is shown with.
func SetReleaseNameTx(ctx context.Context, tx *sql.Tx, fileID int64, release string) error {
	_, err := tx.ExecContext(ctx, "UPDATE files SET release_name = ? WHERE id = ?", release, fileID)
	return err
}
