package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// AdoptedVersionIDs lists the Versions that hold at least one present File
// moved over from another app or adopted from disk.
func AdoptedVersionIDs(ctx context.Context, q database.Lister) ([]int64, error) {
	rows, err := q.QueryContext(ctx, "SELECT DISTINCT version_id FROM files WHERE origin = 'adopted' AND state = 'present'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
