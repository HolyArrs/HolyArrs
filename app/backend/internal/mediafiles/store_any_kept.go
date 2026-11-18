package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// AnyKept reports whether the Library holds any title that is not removed.
func AnyKept(ctx context.Context, q database.Querier) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM titles WHERE state != 'removed')").Scan(&ok)
	return ok, err
}
