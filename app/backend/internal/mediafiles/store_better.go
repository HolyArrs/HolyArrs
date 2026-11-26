package mediafiles

import (
	"context"
	"database/sql"
	"strings"
)

// BetterWanted lists the episodes (kind "show", item ids) or movies (kind
// "movie", title ids) whose main-Version Slot holds a file a better copy
// is still wanted for, of titles kept up to date or not as kept says.
func BetterWanted(ctx context.Context, db *sql.DB, kind string, kept bool) ([]int64, error) {
	col, order := "i.id", "t.sort_name, json_extract(i.numbers, '$.season'), json_extract(i.numbers, '$.episode'), i.id"
	if kind == "movie" {
		col, order = "t.id", "t.sort_name, t.id"
	}
	keep := 0
	if kept {
		keep = 1
	}
	return int64s(db.QueryContext(ctx, `SELECT `+col+` FROM slots s JOIN versions v ON v.id = s.version_id AND v.is_main = 1
		JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE s.state = 'better_wanted' AND t.kind = ? AND t.state != 'removed' AND t.keep_up_to_date = ?
		GROUP BY `+col+` ORDER BY `+order, kind, keep))
}

// MoviesOfFiles lists the movies the files belong to.
func MoviesOfFiles(ctx context.Context, db *sql.DB, fileIDs []int64) ([]int64, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(fileIDs))
	for i, id := range fileIDs {
		args[i] = id
	}
	return int64s(db.QueryContext(ctx, `SELECT DISTINCT v.title_id FROM files f JOIN versions v ON v.id = f.version_id
		JOIN titles t ON t.id = v.title_id WHERE t.kind = 'movie' AND t.state != 'removed' AND f.id IN (`+
		strings.TrimSuffix(strings.Repeat("?,", len(fileIDs)), ",")+`)`, args...))
}

// int64s reads one id column.
func int64s(rows *sql.Rows, err error) ([]int64, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
