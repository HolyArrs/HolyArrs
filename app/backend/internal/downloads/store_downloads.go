package downloads

import (
	"context"
	"database/sql"
)

// totalSpeeds adds up the speeds of every downloading row.
func totalSpeeds(ctx context.Context, db *sql.DB) (SpeedSample, error) {
	var s SpeedSample
	err := db.QueryRowContext(ctx, "SELECT COALESCE(SUM(speed),0), COALESCE(SUM(up_speed),0) FROM downloads WHERE state = 'downloading'").Scan(&s.Down, &s.Up)
	return s, err
}
