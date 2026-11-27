package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// MainVersionID is the id of a title's main Version.
func MainVersionID(ctx context.Context, q database.Querier, titleID int64) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM versions WHERE title_id = ? AND is_main = 1 ORDER BY id LIMIT 1`, titleID).Scan(&id)
	return id, err
}

// SlotIDOfItem is the id of the Slot a Version holds for one Item, and
// sql.ErrNoRows when that Version holds no Slot for the Item at all.
func SlotIDOfItem(ctx context.Context, q database.Querier, itemID, versionID int64) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM slots WHERE item_id = ? AND version_id = ? AND state != 'removed'`, itemID, versionID).Scan(&id)
	return id, err
}

// FileIDOnPath is the id of the file a Version already holds at a path
// relative to the Library folder, and sql.ErrNoRows when it holds none there.
// A file a scan recorded and one adopted by hand are the same row, so a
// second pass over the same folder finds the row instead of adding another.
func FileIDOnPath(ctx context.Context, q database.Querier, versionID int64, rel string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM files WHERE version_id = ? AND relative_path = ? AND state != 'gone'`, versionID, rel).Scan(&id)
	return id, err
}
