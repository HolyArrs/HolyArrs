package mediafiles

import (
	"context"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TitleOnDisk is a movie or show with files in the Library: its TMDB id
// and the disk its files use.
type TitleOnDisk struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Year   int    `json:"year,omitempty"`
	TMDBID string `json:"tmdb_id"`
	Bytes  int64  `json:"bytes"`
	Files  int    `json:"files"`
}

// TitlesOnDisk lists the movies and shows that have files and a TMDB id,
// biggest first.
func TitlesOnDisk(ctx context.Context, db database.Lister) ([]TitleOnDisk, error) {
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.kind, t.name, COALESCE(t.year, 0), json_extract(t.source_ids, '$.tmdb'), SUM(f.size), COUNT(f.id)
		FROM titles t JOIN versions v ON v.title_id = t.id JOIN files f ON f.version_id = v.id
		WHERE t.kind IN ('movie', 'show') AND t.state != 'removed' AND COALESCE(json_extract(t.source_ids, '$.tmdb'), '') != ''
		GROUP BY t.id ORDER BY SUM(f.size) DESC, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TitleOnDisk{}
	for rows.Next() {
		var x TitleOnDisk
		var tmdb any
		if err := rows.Scan(&x.ID, &x.Kind, &x.Name, &x.Year, &tmdb, &x.Bytes, &x.Files); err != nil {
			return nil, err
		}
		switch v := tmdb.(type) {
		case string:
			x.TMDBID = v
		case int64:
			x.TMDBID = strconv.FormatInt(v, 10)
		case float64:
			x.TMDBID = strconv.FormatInt(int64(v), 10)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
