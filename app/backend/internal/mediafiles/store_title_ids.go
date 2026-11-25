package mediafiles

import (
	"context"
	"encoding/json"

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

// TitleKinds is the kind of each title still in the Library, by id.
func TitleKinds(ctx context.Context, q database.Lister) (map[int64]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, kind FROM titles WHERE state != 'removed'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var kind string
		if err := rows.Scan(&id, &kind); err != nil {
			return nil, err
		}
		out[id] = kind
	}
	return out, rows.Err()
}

// EndedTitles is the set of titles among ids whose show has ended. A row
// that does not scan is left out rather than failing the list.
func EndedTitles(ctx context.Context, q database.Lister, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	in, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, "SELECT id FROM titles WHERE status = 'ended' AND id IN (SELECT value FROM json_each(?))", string(in))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out, nil
}
