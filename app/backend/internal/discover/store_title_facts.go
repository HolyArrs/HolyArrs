package discover

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// titleFacts holds every Title's size, last activity and Versions, read in
// three queries, so listing all Titles does not query them once per Title.
type titleFacts struct {
	sizes    map[int64]int64
	activity map[int64]time.Time
	versions map[int64][]mediafiles.Version
	copies   map[int64]mediafiles.TitleCopy
}

type titleFactsKey struct{}

// WithTitleFacts reads the facts for all Titles and keeps them on ctx for View.
func WithTitleFacts(ctx context.Context, db database.Lister) (context.Context, error) {
	f := &titleFacts{sizes: map[int64]int64{}, activity: map[int64]time.Time{}, versions: map[int64][]mediafiles.Version{}}
	rows, err := db.QueryContext(ctx, `SELECT v.title_id, COALESCE(SUM(f.size), 0) FROM files f JOIN versions v ON v.id = f.version_id GROUP BY v.title_id`)
	if err != nil {
		return ctx, err
	}
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			rows.Close()
			return ctx, err
		}
		f.sizes[id] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ctx, err
	}
	rows, err = db.QueryContext(ctx, `SELECT title_id, max(at) FROM (
		SELECT v.title_id, max(f.added_at) AS at FROM files f JOIN versions v ON v.id = f.version_id GROUP BY v.title_id
		UNION ALL
		SELECT v.title_id, max(d.created_at) FROM downloads d, json_each(d.slot_ids) j JOIN slots s ON s.id = j.value
			JOIN versions v ON v.id = s.version_id GROUP BY v.title_id)
		GROUP BY title_id`)
	if err != nil {
		return ctx, err
	}
	for rows.Next() {
		var id int64
		var at sql.NullString
		if err := rows.Scan(&id, &at); err != nil {
			rows.Close()
			return ctx, err
		}
		if t, err := time.Parse(database.TimeLayout, at.String); at.Valid && err == nil {
			f.activity[id] = t
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ctx, err
	}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "1 = 1 ORDER BY title_id, is_main DESC, id")
	if err != nil {
		return ctx, err
	}
	for _, v := range vs {
		f.versions[v.TitleID] = append(f.versions[v.TitleID], v)
	}
	if f.copies, err = mediafiles.TitleCopies(ctx, db); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, titleFactsKey{}, f), nil
}

func factsOf(ctx context.Context) *titleFacts {
	f, _ := ctx.Value(titleFactsKey{}).(*titleFacts)
	return f
}
