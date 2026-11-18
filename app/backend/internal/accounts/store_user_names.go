package accounts

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// UserNames is the name of every person, by id.
func UserNames(ctx context.Context, q database.Lister) (map[int64]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, name FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var n string
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
