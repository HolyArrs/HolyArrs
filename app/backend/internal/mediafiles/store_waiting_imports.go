package mediafiles

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// WaitingImport is a Slot that waits for a decision on the import of the
// download it is tied to: why, and since when.
type WaitingImport struct {
	SlotID     int64
	DownloadID int64
	Reason     string
	Since      time.Time
}

// WaitingImports lists the Slots that wait for a decision while still tied to
// a download, oldest Slot first.
func WaitingImports(ctx context.Context, db *sql.DB) ([]WaitingImport, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, active_download_id, last_reason, updated_at FROM slots
		WHERE state = ? AND active_download_id IS NOT NULL AND last_reason IS NOT NULL ORDER BY id LIMIT 5000`, string(SlotNeedsADecision))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WaitingImport
	for rows.Next() {
		var w WaitingImport
		var since string
		if err := rows.Scan(&w.SlotID, &w.DownloadID, &w.Reason, &since); err != nil {
			return nil, err
		}
		w.Since, _ = time.Parse(database.TimeLayout, since)
		out = append(out, w)
	}
	return out, rows.Err()
}
