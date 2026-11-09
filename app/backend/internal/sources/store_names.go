package sources

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// CountNamed counts the Sources that carry name (any letter case) other than
// the one with exceptID.
func CountNamed(ctx context.Context, q database.Querier, name string, exceptID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE name = ? COLLATE NOCASE AND id != ?", name, exceptID).Scan(&n)
	return n, err
}

// DisableTx pauses a Source for good and replaces its extra settings, inside
// the caller's transaction.
func DisableTx(tx *sql.Tx, id int64, extra string) error {
	_, err := tx.Exec("UPDATE sources SET state = 'disabled', pause_reason = NULL, paused_until = NULL, extra = ? WHERE id = ?", extra, id)
	return err
}
