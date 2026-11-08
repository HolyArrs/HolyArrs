package accounts

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// keyBy reads the API key behind a hash and its active person into who.
func keyBy(ctx context.Context, q conn, hash string, who *webserver.Caller) (keyID int64, used sql.NullString, scope, perms string, err error) {
	err = q.QueryRowContext(ctx, `SELECT k.id, k.label, k.last_used_at, k.scope, `+userCols+` FROM api_keys k JOIN users u ON u.id = k.owner_user_id
		WHERE k.hash = ? AND u.state = 'active'`, hash).Scan(&keyID, &who.KeyLabel, &used, &scope, &who.UserID, &who.Name, &who.Role, &perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry)
	return keyID, used, scope, perms, err
}

// touchKey records that an API key was used.
func touchKey(ctx context.Context, q conn, at string, keyID int64) error {
	_, err := q.ExecContext(ctx, "UPDATE api_keys SET last_used_at = ? WHERE id = ?", at, keyID)
	return err
}

// insertKey stores a new API key and returns its id.
func insertKey(ctx context.Context, q conn, now, actor string, ownerID int64, label, scopes, hash, hint string) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO api_keys(created_at, created_by, updated_at, updated_by, owner_user_id, label, scope, hash, hint)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, now, actor, now, actor, ownerID, label, scopes, hash, hint)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// listKeyRows lists the keys the person may see: every key for the owner,
// else their own.
func listKeyRows(ctx context.Context, q conn, all bool, userID int64) (*sql.Rows, error) {
	return q.QueryContext(ctx, "SELECT id, label, hint, created_at, last_used_at, scope FROM api_keys WHERE ? OR owner_user_id = ? ORDER BY id",
		all, userID)
}

// keyLabel is the label of an API key, "" when there is none.
func keyLabel(ctx context.Context, q conn, id int64) string {
	var label string
	_ = q.QueryRowContext(ctx, "SELECT label FROM api_keys WHERE id = ?", id).Scan(&label)
	return label
}

// deleteKey removes an API key the person owns, or any key for the owner,
// and returns how many rows went.
func deleteKey(ctx context.Context, q conn, id int64, all bool, userID int64) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM api_keys WHERE id = ? AND (? OR owner_user_id = ?)", id, all, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// deleteKeysOf removes every API key a person owns.
func deleteKeysOf(ctx context.Context, q conn, userID int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM api_keys WHERE owner_user_id = ?", userID)
	return err
}
