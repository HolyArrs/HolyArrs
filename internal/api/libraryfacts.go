package api

import (
	"context"
	"database/sql"
	"time"

	"github.com/CoderLuii/holyarrs/internal/store"
)

// titleSize sums the size of every file kept for a title, across all its
// Versions, for the Library's size column.
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
	rows, err := db.QueryContext(ctx, `SELECT air_date FROM items WHERE title_id = ? AND kind = 'episode' AND air_date IS NOT NULL`, titleID)
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
