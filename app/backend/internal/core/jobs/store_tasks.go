package jobs

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Schedules lists every scheduled kind, by kind.
func Schedules(ctx context.Context, db *sql.DB) ([]Schedule, error) {
	return database.Select[Schedule](ctx, db, "schedules", "1 = 1 ORDER BY kind")
}

// LastFinished reads the start and end of a kind's newest finished Job, as
// the Schedules page reads it. Both are "" when it never ran.
func LastFinished(ctx context.Context, db *sql.DB, kind string) (started, ended string, err error) {
	var s, e sql.NullString
	err = db.QueryRowContext(ctx, `SELECT started_at, ended_at FROM jobs WHERE kind = ? AND state IN ('done', 'failed', 'cancelled')
		ORDER BY id DESC LIMIT 1`, kind).Scan(&s, &e)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return s.String, e.String, err
}

// CountLive counts the Jobs in ids that are still queued or running.
func CountLive(ctx context.Context, db *sql.DB, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	var n int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE state IN ('queued', 'running') AND id IN ("+
		strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+")", args...).Scan(&n)
	return n, err
}
