package downloads

import (
	"context"
	"database/sql"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// adoptID moves a download found by its job's second id over to the id the
// client lists it by, so later calls and the outside-items list use that id.
// On a failed write the row keeps its id and is found by the second id again.
func (m *Manager) adoptID(ctx context.Context, d domain.Download, j Job) domain.Download {
	if j.ID == "" || j.ID == d.ExternalID || j.AltID != d.ExternalID {
		return d
	}
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE downloads SET external_id = ? WHERE id = ? AND external_id = ?", j.ID, d.ID, d.ExternalID)
		return err
	})
	if err == nil {
		d.ExternalID = j.ID
	}
	return d
}
