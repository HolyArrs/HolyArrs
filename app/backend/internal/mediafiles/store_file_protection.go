package mediafiles

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ProtectedAt reports whether a File of the Version at one of the paths is
// protected, so the File that replaces it can keep that choice.
func ProtectedAt(ctx context.Context, q database.Querier, versionID int64, paths ...string) (bool, error) {
	var n int
	for _, p := range paths {
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ? AND protected = 1", versionID, p).Scan(&n); err != nil || n > 0 {
			return n > 0, err
		}
	}
	return false, nil
}

// TakeNotesAtTx removes found, unlinked, or non-present file rows of a Version
// at a path so an import can record the file it put there, and reports whether
// any removed row was protected so the new file keeps that protection.
func TakeNotesAtTx(ctx context.Context, tx *sql.Tx, versionID int64, path string) (bool, error) {
	const notes = `version_id = ? AND relative_path = ? AND (slot_ids = '[]' OR origin = 'found' OR state != 'present')`
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE protected = 1 AND "+notes, versionID, path).Scan(&n); err != nil {
		return false, err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM files WHERE "+notes, versionID, path)
	return n > 0, err
}

// ProtectAt marks the present Files of the Version at the path protected.
func ProtectAt(ctx context.Context, q database.Execer, versionID int64, path string) error {
	_, err := q.ExecContext(ctx, "UPDATE files SET protected = 1 WHERE version_id = ? AND relative_path = ? AND state = 'present'", versionID, path)
	return err
}
