package alerts

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// listAll reads every Alert by name.
func listAll(ctx context.Context, q database.Lister) ([]domain.Alert, error) {
	return database.Select[domain.Alert](ctx, q, "alerts", "1=1 ORDER BY name")
}

// listEnabled reads the Alerts that are turned on, oldest first.
func listEnabled(ctx context.Context, q database.Lister) ([]domain.Alert, error) {
	return database.Select[domain.Alert](ctx, q, "alerts", "enabled = 1 ORDER BY id")
}

// get reads one Alert.
func get(ctx context.Context, q database.Querier, id int64) (domain.Alert, error) {
	var a domain.Alert
	err := database.Get(ctx, q, "alerts", id, &a)
	return a, err
}

// markFailed keeps the reason of the last failed send on the Alert.
func markFailed(tx *sql.Tx, id int64, msg string) error {
	_, err := tx.Exec("UPDATE alerts SET last_error = ? WHERE id = ?", msg, id)
	return err
}

// markSent keeps the time of the last good send and clears the last reason.
func markSent(tx *sql.Tx, id int64, now string) error {
	_, err := tx.Exec("UPDATE alerts SET last_sent_at = ?, last_error = '' WHERE id = ?", now, id)
	return err
}

// nameUsed refuses a name another Alert already has, whatever its case.
func nameUsed(tx *sql.Tx, name string, id int64) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM alerts WHERE name = ? COLLATE NOCASE AND id != ?", name, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrNameUsed
	}
	return nil
}

// deleteRow removes one Alert row; database.ErrNotFound when none was there.
func deleteRow(tx *sql.Tx, id int64) error {
	res, err := tx.Exec("DELETE FROM alerts WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return database.ErrNotFound
	}
	return nil
}
