package downloads

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// orphanCandidates lists the running, finished or seeding downloads whose Downloader
// was removed, oldest first.
func orphanCandidates(ctx context.Context, q database.Lister) ([]orphan, error) {
	rows, err := q.QueryContext(ctx, `SELECT d.id, d.downloader_id, d.external_id, d.state FROM downloads d
		LEFT JOIN downloaders r ON r.id = d.downloader_id
		WHERE (r.id IS NULL OR r.removed = 1) AND (d.state IN (`+activeStates+`) OR d.state IN ('finished', 'seeding'))
		ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.id, &o.downloader, &o.hash, &o.state); err != nil {
			return out, err
		}
		o.hash = strings.ToLower(o.hash)
		out = append(out, o)
	}
	return out, rows.Err()
}

// relinkTx moves one download to the Downloader to, unless a download of to
// already has that torrent. It reports whether the row moved.
func relinkTx(ctx context.Context, tx *sql.Tx, o orphan, to int64, now time.Time) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE downloads SET downloader_id = ?, updated_at = ? WHERE id = ? AND downloader_id = ?
		AND NOT EXISTS (SELECT 1 FROM downloads x WHERE x.downloader_id = ? AND lower(x.external_id) = ?)`,
		to, database.FormatTime(now), o.id, o.downloader, to, o.hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
