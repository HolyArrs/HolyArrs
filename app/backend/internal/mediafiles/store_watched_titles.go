package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TitleWithEpisodes is a movie or show with present files, and for a show
// the season and episode numbers that have a file.
type TitleWithEpisodes struct {
	ID       int64
	Kind     string
	Name     string
	Year     int
	Bytes    int64
	Episodes [][2]int
}

// TitlesWithFilesUnprotected lists the movies and shows with present files
// and no protected copy, biggest first, each show with its episodes on disk.
func TitlesWithFilesUnprotected(ctx context.Context, db database.Lister) ([]TitleWithEpisodes, error) {
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.kind, t.name, COALESCE(t.year, 0), SUM(f.size)
		FROM titles t JOIN versions v ON v.title_id = t.id JOIN files f ON f.version_id = v.id AND f.state = 'present'
		WHERE t.kind IN ('movie', 'show') AND t.state != 'removed'
		AND NOT EXISTS (SELECT 1 FROM versions pv JOIN files pf ON pf.version_id = pv.id WHERE pv.title_id = t.id AND pf.protected = 1 AND pf.state = 'present')
		GROUP BY t.id ORDER BY SUM(f.size) DESC, t.id`)
	if err != nil {
		return nil, err
	}
	out := []TitleWithEpisodes{}
	at := map[int64]int{}
	for rows.Next() {
		var x TitleWithEpisodes
		if err := rows.Scan(&x.ID, &x.Kind, &x.Name, &x.Year, &x.Bytes); err != nil {
			rows.Close()
			return nil, err
		}
		at[x.ID] = len(out)
		out = append(out, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	eps, err := db.QueryContext(ctx, `SELECT DISTINCT i.title_id, COALESCE(json_extract(i.numbers, '$.season'), 0), COALESCE(json_extract(i.numbers, '$.episode'), 0)
		FROM slots s JOIN items i ON i.id = s.item_id JOIN files f ON f.id = s.file_id AND f.state = 'present'
		WHERE i.kind = 'episode' AND COALESCE(json_extract(i.numbers, '$.season'), 0) > 0`)
	if err != nil {
		return nil, err
	}
	defer eps.Close()
	for eps.Next() {
		var title int64
		var se [2]int
		if err := eps.Scan(&title, &se[0], &se[1]); err != nil {
			return nil, err
		}
		if i, ok := at[title]; ok {
			out[i].Episodes = append(out[i].Episodes, se)
		}
	}
	return out, eps.Err()
}
