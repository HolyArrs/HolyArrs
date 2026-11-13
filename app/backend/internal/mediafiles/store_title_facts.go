package mediafiles

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TitleActivity is the newest grab (a Download for one of its Slots) or
// import (a file in one of its Versions); nil when there is none.
func TitleActivity(ctx context.Context, db database.Lister, titleID int64) (*time.Time, error) {
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
	t, err := time.Parse(database.TimeLayout, at.String)
	if err != nil {
		return nil, nil
	}
	return &t, nil
}

// TitleSize sums the size of every file kept for a title, across all its
// Versions, for the Library's size column.
func TitleSize(ctx context.Context, db database.Lister, titleID int64) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(SUM(size), 0) FROM files
		WHERE state = 'present' AND version_id IN (SELECT id FROM versions WHERE title_id = ?)`, titleID)
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
