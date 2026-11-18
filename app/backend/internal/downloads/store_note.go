package downloads

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// noteCause stores a Test's line as the last error of a Downloader that
// failed, so health names the cause the Test found.
func (m *Manager) noteCause(ctx context.Context, id int64, line string) {
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET last_error = ? WHERE id = ? AND state <> 'ok' AND last_error <> ''", line, id)
		return err
	})
}

// noteCategory records a missing category as the last error only, with no
// back-off and no change of state.
func (m *Manager) noteCategory(ctx context.Context, id int64, cause error) {
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET last_error = ? WHERE id = ?", firstLine(cause.Error()), id)
		return err
	})
}

// categoryFound is a check's good answer: every category is there, so a stored
// missing-category line goes too, then the back-off ends.
func (m *Manager) categoryFound(ctx context.Context, d domain.Downloader) {
	if doctor.OfText(d.LastError) == doctor.Category {
		_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE downloaders SET last_error = '' WHERE id = ?", d.ID)
			return err
		})
		d.LastError = ""
		m.publish("downloader", map[string]any{"id": d.ID, "state": d.State})
	}
	m.downloaderOK(ctx, d)
}
