package requests

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Statements of ask_details.go. The text is the exact SQL the server runs.
const (
	fillCandidatesSQL = "SELECT id FROM asks WHERE kind IN ('movie', 'show') AND ref != '' AND (name = '' OR name GLOB 'TMDB [0-9]*' OR name GLOB 'TVDB [0-9]*' OR name GLOB 'IMDB *' OR (poster = '' AND year IS NULL)) ORDER BY id"
	setAskDetailsSQL  = "UPDATE asks SET name = ?, year = ?, poster = ?, updated_at = ? WHERE id = ? AND name = ?"
)

func fillCandidates(ctx context.Context, q *sql.DB) ([]int64, error) {
	rows, err := q.QueryContext(ctx, fillCandidatesSQL)
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

// setAskDetails writes the details unless the ask's name changed meanwhile.
func setAskDetails(ctx context.Context, st *database.Store, id int64, was, name string, year *int, poster string, now time.Time) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, setAskDetailsSQL, name, year, poster, database.FormatTime(now), id, was)
		return err
	})
}
