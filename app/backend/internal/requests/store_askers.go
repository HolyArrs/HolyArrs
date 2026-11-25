package requests

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// AskersByTitle lists, per Library title, the people who asked for it
// (asks turned down left out).
func AskersByTitle(ctx context.Context, db database.Lister) (map[int64][]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT a.title_id, p.user_id FROM asks a JOIN ask_people p ON p.ask_id = a.id
		WHERE a.title_id IS NOT NULL AND a.state != 'turned_down' ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]int64{}
	for rows.Next() {
		var title, user int64
		if err := rows.Scan(&title, &user); err != nil {
			return nil, err
		}
		out[title] = append(out[title], user)
	}
	return out, rows.Err()
}
