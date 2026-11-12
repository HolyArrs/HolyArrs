package downloads

import (
	"database/sql"
	"errors"
)

// RunningTx reports whether a download is still in its Downloader's hands
// (sent, queued, downloading, stalled or paused). A finished, failed,
// vanished or cleaned download, or one that is gone, is not.
func RunningTx(tx *sql.Tx, id int64) (bool, error) {
	var n int
	err := tx.QueryRow("SELECT count(*) FROM downloads WHERE id = ? AND state IN ("+activeStates+")", id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return n > 0, err
}
