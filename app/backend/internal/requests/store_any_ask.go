package requests

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// AnyAsk reports whether anything was ever asked for.
func AnyAsk(ctx context.Context, q database.Querier) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM asks)").Scan(&ok)
	return ok, err
}
