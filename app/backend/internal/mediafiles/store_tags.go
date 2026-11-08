package mediafiles

import (
	"context"
	"database/sql"
)

// TitleTagUses counts, per tag label, the titles (not removed) that carry it.
// A row saved without tags may hold JSON null rather than a list.
func TitleTagUses(ctx context.Context, db *sql.DB) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, "SELECT j.value FROM titles x, json_each(x.tags) j WHERE json_type(x.tags) = 'array' AND x.state != 'removed'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		out[label]++
	}
	return out, rows.Err()
}
