package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TitleIDs is a Library movie or show with its TMDB and TVDB ids, empty
// when the title has none.
type TitleIDs struct {
	ID   int64
	Kind string
	TMDB string
	TVDB string
}

const (
	liveTitleIDsSQL = `SELECT id, kind, COALESCE(CAST(json_extract(source_ids, '$.tmdb') AS TEXT), ''),
		COALESCE(CAST(json_extract(source_ids, '$.tvdb') AS TEXT), '')
		FROM titles WHERE kind IN ('movie', 'show') AND state != 'removed'`
	removedTitleIDsSQL = "SELECT id FROM titles WHERE state = 'removed'"
)

// LiveTitleIDs lists the movies and shows still in the Library, by id. A
// titleID above zero keeps only that title.
func LiveTitleIDs(ctx context.Context, q database.Lister, titleID int64) ([]TitleIDs, error) {
	query, args := liveTitleIDsSQL, []any{}
	if titleID > 0 {
		query, args = query+" AND id = ?", append(args, titleID)
	}
	rows, err := q.QueryContext(ctx, query+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TitleIDs
	for rows.Next() {
		var t TitleIDs
		if err := rows.Scan(&t.ID, &t.Kind, &t.TMDB, &t.TVDB); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RemovedTitleIDs is the set of titles deleted from the Library.
func RemovedTitleIDs(ctx context.Context, q database.Lister) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx, removedTitleIDsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
