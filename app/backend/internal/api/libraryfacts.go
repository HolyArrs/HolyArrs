package api

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// titleSize sums the size of every file kept for a title, across all its
// Versions, for the Library's size column.
// titleActivity is the newest grab (a Download for one of its Slots) or
// import (a file in one of its Versions); nil when there is none.
func titleActivity(ctx context.Context, db store.Lister, titleID int64) (*time.Time, error) {
	rows, err := db.QueryContext(ctx, `SELECT max(at) FROM (
		SELECT max(added_at) AS at FROM files WHERE version_id IN (SELECT id FROM versions WHERE title_id = ?)
		UNION ALL
		SELECT max(d.created_at) FROM downloads d WHERE EXISTS (SELECT 1 FROM json_each(d.slot_ids) j JOIN slots s ON s.id = j.value
			WHERE s.version_id IN (SELECT id FROM versions WHERE title_id = ?)))`, titleID, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var at sql.NullString
	if rows.Next() {
		if err := rows.Scan(&at); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil || !at.Valid {
		return nil, err
	}
	t, err := time.Parse(store.TimeLayout, at.String)
	if err != nil {
		return nil, nil
	}
	return &t, nil
}

func titleSize(ctx context.Context, db store.Lister, titleID int64) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(SUM(size), 0) FROM files
		WHERE version_id IN (SELECT id FROM versions WHERE title_id = ?)`, titleID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var n int64
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
	}
	return n, rows.Err()
}

// lastAired is the newest air date of a show's episodes on or before now;
// nil when nothing has aired yet.
func lastAired(ctx context.Context, db store.Lister, titleID int64, now time.Time) (*time.Time, error) {
	// Stored times share one fixed-width UTC layout, so SQLite can pick the
	// newest aired one instead of the server parsing every episode.
	rows, err := db.QueryContext(ctx, `SELECT max(air_date) FROM items WHERE title_id = ? AND kind = 'episode' AND air_date IS NOT NULL AND air_date <= ?`, titleID, store.FormatTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var last *time.Time
	for rows.Next() {
		var air sql.NullString
		if err := rows.Scan(&air); err != nil {
			return nil, err
		}
		at, err := time.Parse(store.TimeLayout, air.String)
		if err != nil || at.After(now) {
			continue
		}
		if last == nil || at.After(*last) {
			a := at
			last = &a
		}
	}
	return last, rows.Err()
}
