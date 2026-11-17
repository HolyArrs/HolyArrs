package downloads

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// clearLater records a failed clear and when to try it next; it returns how
// many tries have failed so far.
func (m *Manager) clearLater(ctx context.Context, id int64, cause error) int {
	tries := 0
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRow("SELECT clear_tries FROM downloads WHERE id = ?", id).Scan(&tries); err != nil {
			return err
		}
		tries++
		_, err := tx.Exec("UPDATE downloads SET clear_retry_at = ?, clear_tries = ?, clear_error = ? WHERE id = ?",
			database.FormatTime(m.now().Add(clearWaitAfter(tries))), tries, firstLine(cause.Error()), id)
		return err
	})
	return tries
}

// clearDone ends any wait to clear the download.
func (m *Manager) clearDone(ctx context.Context, id int64) {
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET clear_retry_at = NULL, clear_tries = 0, clear_error = '' WHERE id = ? AND (clear_retry_at IS NOT NULL OR clear_tries > 0)", id)
		return err
	})
}

// ClearWaits reads every download waiting to be cleared, by download id.
func ClearWaits(ctx context.Context, db *sql.DB) map[int64]ClearWait {
	out := map[int64]ClearWait{}
	rows, err := db.QueryContext(ctx, "SELECT id, clear_retry_at, clear_tries, clear_error FROM downloads WHERE clear_retry_at IS NOT NULL")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var at string
		var w ClearWait
		if rows.Scan(&id, &at, &w.Tries, &w.Reason) != nil {
			continue
		}
		w.NextTry, _ = time.Parse(database.TimeLayout, at)
		out[id] = w
	}
	return out
}

// dueClears lists the downloads whose wait to clear is over, oldest first.
func (m *Manager) dueClears(ctx context.Context) []dueClear {
	rows, err := m.Store.Read().QueryContext(ctx, "SELECT id, state FROM downloads WHERE clear_retry_at IS NOT NULL AND clear_retry_at <= ? ORDER BY clear_retry_at, id", database.FormatTime(m.now()))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var list []dueClear
	for rows.Next() {
		var d dueClear
		if rows.Scan(&d.id, &d.state) == nil {
			list = append(list, d)
		}
	}
	return list
}

// dueClear is a download whose wait to clear is over.
type dueClear struct {
	id    int64
	state string
}

// updateReused writes a new grab over an old download row; any wait to clear
// the old download never carries over to the new one.
func updateReused(tx *sql.Tx, dl *domain.Download, by domain.Actor) error {
	if err := database.Update(tx, "downloads", dl, by); err != nil {
		return err
	}
	_, err := tx.Exec("UPDATE downloads SET clear_retry_at = NULL, clear_tries = 0, clear_error = '' WHERE id = ?", dl.ID)
	return err
}

// dueNow makes every waiting clear due at once (Clear now).
func (m *Manager) dueNow(ctx context.Context) {
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET clear_retry_at = ? WHERE clear_retry_at IS NOT NULL", database.FormatTime(m.now()))
		return err
	})
}

// waitingClears counts the downloads still waiting to be cleared.
func (m *Manager) waitingClears(ctx context.Context) int {
	var n int
	_ = m.Store.Read().QueryRowContext(ctx, "SELECT COUNT(*) FROM downloads WHERE clear_retry_at IS NOT NULL").Scan(&n)
	return n
}
