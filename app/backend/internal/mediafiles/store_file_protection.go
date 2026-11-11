package mediafiles

import (
	"context"

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

// ProtectAt marks the present Files of the Version at the path protected.
func ProtectAt(ctx context.Context, q database.Execer, versionID int64, path string) error {
	_, err := q.ExecContext(ctx, "UPDATE files SET protected = 1 WHERE version_id = ? AND relative_path = ? AND state = 'present'", versionID, path)
	return err
}
