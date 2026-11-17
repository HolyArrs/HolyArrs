package downloads

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// PartCounts counts the Downloaders that are not removed, those that are
// on, and those on whose state already says they fail (not ok, working or
// enabled), the way Home counts them.
func PartCounts(ctx context.Context, q database.Querier) (total, on, failing int, err error) {
	err = q.QueryRowContext(ctx, `SELECT count(*),
		count(*) FILTER (WHERE state != 'disabled'),
		count(*) FILTER (WHERE state NOT IN ('ok', 'working', 'enabled', 'disabled'))
		FROM downloaders WHERE removed = 0 AND name NOT LIKE '% (removed %)'`).Scan(&total, &on, &failing)
	return total, on, failing, err
}
