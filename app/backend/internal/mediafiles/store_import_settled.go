package mediafiles

import (
	"context"
	"database/sql"
)

// ImportSettled reports whether the newest import of the download is done
// and nothing about it is still at work or waiting for a person: no import
// planned, approved or placing, and no Slot of it importing or needing a
// decision.
func ImportSettled(ctx context.Context, db *sql.DB, downloadID int64) (bool, error) {
	var ok bool
	err := db.QueryRowContext(ctx, `SELECT COALESCE(
		(SELECT i.state FROM imports i WHERE i.download_id = ?1 ORDER BY i.id DESC LIMIT 1) = 'done'
		AND NOT EXISTS (SELECT 1 FROM imports i WHERE i.download_id = ?1 AND i.state IN ('planned', 'waiting_for_you', 'approved', 'placing'))
		AND NOT EXISTS (SELECT 1 FROM slots s WHERE s.active_download_id = ?1 AND s.state IN ('importing', 'needs_a_decision')), 0)`,
		downloadID).Scan(&ok)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return ok, err
}
