package downloads

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// downloadStateTx reads a download's state inside a write.
func downloadStateTx(tx *sql.Tx, id int64) (string, error) {
	var cur string
	err := tx.QueryRow("SELECT state FROM downloads WHERE id = ?", id).Scan(&cur)
	return cur, err
}

// finishedRows lists the finished torrent downloads HolyArrs still shows,
// by Downloader, that no removal is pending for.
func finishedRows(ctx context.Context, db *sql.DB) map[int64][]domain.Download {
	out := map[int64][]domain.Download{}
	list, err := database.Select[domain.Download](ctx, db, "downloads", `state IN ('finished', 'seeding') AND removal_pending IS NULL AND clear_retry_at IS NULL
		AND downloader_id IN (SELECT id FROM downloaders WHERE protocol = 'torrent' AND removed = 0) ORDER BY id`)
	if err != nil {
		return out
	}
	for _, d := range list {
		out[d.DownloaderID] = append(out[d.DownloaderID], d)
	}
	return out
}
