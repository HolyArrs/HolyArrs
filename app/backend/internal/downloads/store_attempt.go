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
