package secretsfile

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The move credential statements are this package's own. The lookups of an
// existing Source, Downloader, Media server or Alert and the paused marking
// of a new one belong to the migrate orchestrator: they run inside the one
// transaction that makes the item, so they stay here, byte for byte.

func insertMoveCredential(ctx context.Context, tx *sql.Tx, kind string, secretID int64, createdAt, createdBy string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO move_credentials (kind, secret_id, created_at, created_by) VALUES (?, ?, ?, ?)",
		kind, secretID, createdAt, createdBy)
	return err
}

func moveCredentialSecret(ctx context.Context, q database.Querier, kind string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT secret_id FROM move_credentials WHERE kind = ?", kind).Scan(&id)
	return id, err
}

func countMoveCredentials(ctx context.Context, q database.Querier, kind string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM move_credentials WHERE kind = ?", kind).Scan(&n)
	return n, err
}

func deleteMoveCredential(ctx context.Context, tx *sql.Tx, kind string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM move_credentials WHERE kind = ?", kind)
	return err
}

func sourceIDByName(ctx context.Context, q database.Querier, name string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT id FROM sources WHERE name = ? COLLATE NOCASE", name).Scan(&id)
	return id, err
}

func downloaderID(ctx context.Context, q database.Querier, adapter, addr, name string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM downloaders WHERE removed = 0 AND adapter = ? AND (url = ? OR name = ? COLLATE NOCASE)
		ORDER BY url = ? DESC, id LIMIT 1`, adapter, addr, name, addr).Scan(&id)
	return id, err
}

func mediaServerID(ctx context.Context, q database.Querier, kind, addr, name string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM media_servers WHERE kind = ? AND (url = ? OR name = ? COLLATE NOCASE)
		ORDER BY url = ? DESC, id LIMIT 1`, kind, addr, name, addr).Scan(&id)
	return id, err
}

func alertID(ctx context.Context, q database.Querier, name, kind string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT id FROM alerts WHERE name = ? COLLATE NOCASE AND kind = ?", name, kind).Scan(&id)
	return id, err
}

// markPaused pauses the new item and records it as the import's.
func markPaused(tx *sql.Tx, table string, id int64) error {
	if _, err := tx.Exec("UPDATE "+table+" SET state = 'disabled' WHERE id = ?", id); err != nil {
		return err
	}
	_, err := tx.Exec("INSERT OR REPLACE INTO import_created (tbl, item_id, live) VALUES (?, ?, 1)", table, id)
	return err
}

// importMade counts the live import marks for an item.
func importMade(ctx context.Context, q database.Querier, table string, id int64) int {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM import_created WHERE tbl = ? AND item_id = ? AND live = 1", table, id).Scan(&n)
	return n
}
