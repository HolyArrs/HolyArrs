package jobs

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// LiveIDsWithKeyPrefix lists the ids of queued and running jobs whose
// dedupe key starts with prefix, oldest first, so a module can stop the
// work it asked for.
func LiveIDsWithKeyPrefix(ctx context.Context, q database.Lister, prefix string) ([]int64, error) {
	rows, err := q.QueryContext(ctx, "SELECT id FROM jobs WHERE substr(dedupe_key, 1, ?) = ? AND state IN ('queued', 'running') ORDER BY id", len(prefix), prefix)
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
