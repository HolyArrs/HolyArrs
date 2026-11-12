package downloads

import (
	"context"
	"database/sql"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// seedCandidates lists the finished torrents HolyArrs grabbed, on Downloaders
// set to remove after seeding. Whether their import is done is asked of
// mediafiles.ImportSettled.
func seedCandidates(ctx context.Context, db *sql.DB) ([]domain.Download, error) {
	return database.Select[domain.Download](ctx, db, "downloads", `id IN (
		SELECT d.id FROM downloads d JOIN downloaders r ON r.id = d.downloader_id
		WHERE d.state = 'finished' AND d.release_id IS NOT NULL AND d.removal_pending IS NULL
		  AND r.protocol = 'torrent' AND r.after_import = 'remove' AND r.removed = 0 AND r.state != 'disabled'
	) ORDER BY downloader_id, id`)
}

// markSeeded ends a finished download as cleaned, with the client removal
// queued in removal_pending, so a client that is down is asked again on a
// later sync. It reports false when the row is no longer finished.
func markSeeded(tx *sql.Tx, next *domain.Download, deleteFiles bool) (bool, error) {
	if err := database.UpdateWhere(tx, "downloads", next, domain.System, "state = 'finished'"); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	_, err := tx.Exec("UPDATE downloads SET removal_pending = ? WHERE id = ?", deleteFiles, next.ID)
	return err == nil, err
}

// deadCandidates lists the torrents still downloading on Downloaders that may
// change what they hold.
func deadCandidates(ctx context.Context, db *sql.DB) ([]domain.Download, error) {
	return database.Select[domain.Download](ctx, db, "downloads", `state IN ('downloading', 'stalled') AND downloader_id IN (
		SELECT id FROM downloaders WHERE protocol = 'torrent' AND removed = 0) ORDER BY downloader_id, id`)
}

// categoryMoved reports whether the download was already moved to its
// category after import.
func (m *Manager) categoryMoved(ctx context.Context, id int64) (bool, error) {
	var at sql.NullString
	err := m.Store.Read().QueryRowContext(ctx, "SELECT category_moved_at FROM downloads WHERE id = ?", id).Scan(&at)
	return at.Valid, err
}

// markCategoryMoved records that the download was moved after import.
func (m *Manager) markCategoryMoved(ctx context.Context, id int64) error {
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET category_moved_at = ? WHERE id = ? AND category_moved_at IS NULL", database.FormatTime(m.now()), id)
		return err
	})
}
