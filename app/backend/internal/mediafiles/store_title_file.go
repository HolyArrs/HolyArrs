package mediafiles

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TitleFile reads one File of a title that is still known on disk (present
// or missing). ok is false when the File is not that title's, or is already
// gone or replaced.
func TitleFile(ctx context.Context, db database.Lister, titleID, fileID int64) (File, bool, error) {
	fs, err := database.Select[File](ctx, db, "files",
		"id = ? AND state IN ('present', 'missing') AND version_id IN (SELECT id FROM versions WHERE title_id = ?)", fileID, titleID)
	if err != nil || len(fs) == 0 {
		return File{}, false, err
	}
	return fs[0], true, nil
}

// EmptySlotsOfFileTx makes the Slots a deleted File filled want a copy
// again, with the reason. The File's own row is left to the caller.
func EmptySlotsOfFileTx(ctx context.Context, tx *sql.Tx, fileID int64, reason string, now time.Time, by database.Actor) error {
	_, err := tx.ExecContext(ctx, `UPDATE slots SET file_id = NULL, good_enough = 0, state = 'wanted', last_reason = ?, updated_at = ?, updated_by = ?
		WHERE file_id = ? AND state IN ('in_library', 'better_wanted')`, reason, database.FormatTime(now), actorJSON(by), fileID)
	return err
}
