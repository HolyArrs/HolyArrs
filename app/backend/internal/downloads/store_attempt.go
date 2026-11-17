package downloads

import (
	"context"
	"database/sql"
	"time"
)

// resolveAttempt marks a pending Downloader attempt resolved. It returns
// ErrAttemptStopped when the attempt was no longer pending.
func (m *Manager) resolveAttempt(ctx context.Context, id int64) error {
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, "UPDATE downloader_attempts SET state = 'resolved', updated_at = ? WHERE id = ? AND state = 'pending'", m.now().Format(time.RFC3339Nano), id)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e == nil && n != 1 {
			return ErrAttemptStopped
		}
		return e
	})
}

// dueAttempts lists the Slot and Release of pending sends made at or before
// cutoff, oldest first.
func (m *Manager) dueAttempts(ctx context.Context, cutoff string, limit int) ([][2]int64, error) {
	rows, err := m.Store.Read().QueryContext(ctx, `SELECT slot_id, release_id FROM downloader_attempts
		WHERE state = 'pending' AND created_at <= ? ORDER BY id LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]int64
	for rows.Next() {
		var p [2]int64
		if err := rows.Scan(&p[0], &p[1]); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
