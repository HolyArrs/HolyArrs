package accounts

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// keyBy reads the API key behind a hash and its active person into who.
func keyBy(ctx context.Context, q conn, hash string, who *webserver.Caller) (keyID int64, used sql.NullString, scope, groups, perms string, err error) {
	err = q.QueryRowContext(ctx, `SELECT k.id, k.label, k.last_used_at, k.scope, k.mcp_groups, `+userCols+` FROM api_keys k JOIN users u ON u.id = k.owner_user_id
		WHERE k.hash = ? AND u.state = 'active'`, hash).Scan(&keyID, &who.KeyLabel, &used, &scope, &groups, &who.UserID, &who.Name, &who.Role, &perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry)
	return keyID, used, scope, groups, perms, err
}

// touchKey records that an API key was used.
func touchKey(ctx context.Context, q conn, at string, keyID int64) error {
	_, err := q.ExecContext(ctx, "UPDATE api_keys SET last_used_at = ? WHERE id = ?", at, keyID)
	return err
}

// insertKey stores a new API key and returns its id.
// groups is the key's mcp_groups JSON; "" keeps the column default.
func insertKey(ctx context.Context, q conn, now, actor string, ownerID int64, label, scopes, groups, hash, hint string) (int64, error) {
	if groups == "" {
		groups = defaultGroupsJSON
	}
	res, err := q.ExecContext(ctx, `INSERT INTO api_keys(created_at, created_by, updated_at, updated_by, owner_user_id, label, scope, mcp_groups, hash, hint)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, now, actor, now, actor, ownerID, label, scopes, groups, hash, hint)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// listKeyRows lists the keys the person may see: every key for the owner,
// else their own.
func listKeyRows(ctx context.Context, q conn, all bool, userID int64) (*sql.Rows, error) {
	return q.QueryContext(ctx, "SELECT id, label, hint, created_at, last_used_at, scope, mcp_groups FROM api_keys WHERE ? OR owner_user_id = ? ORDER BY id",
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

// keyForEdit reads the label, scope list and tick boxes of a key the person
// owns, or of any key for the owner; sql.ErrNoRows when there is none.
func keyForEdit(ctx context.Context, q conn, id int64, all bool, userID int64) (label, scope, groups string, err error) {
	err = q.QueryRowContext(ctx, "SELECT label, scope, mcp_groups FROM api_keys WHERE id = ? AND (? OR owner_user_id = ?)", id, all, userID).Scan(&label, &scope, &groups)
	return label, scope, groups, err
}

// setKeyGroups stores an AI key's tick boxes.
func setKeyGroups(ctx context.Context, q conn, id int64, groups, now, actor string) error {
	_, err := q.ExecContext(ctx, "UPDATE api_keys SET mcp_groups = ?, updated_at = ?, updated_by = ? WHERE id = ?", groups, now, actor, id)
	return err
}

// madeKey is an API key HolyArrs made on its own, without its value.
type madeKey struct {
	id          int64
	hash, scope string
}

// madeKeys lists a person's API keys with a label, made by actor (as stored).
func madeKeys(ctx context.Context, q conn, userID int64, label, actor string) ([]madeKey, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, hash, scope FROM api_keys WHERE owner_user_id = ? AND label = ? AND created_by = ? ORDER BY id",
		userID, label, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []madeKey
	for rows.Next() {
		var k madeKey
		if err := rows.Scan(&k.id, &k.hash, &k.scope); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// madeKeyHash is the hash of key id when it has this label and maker;
// sql.ErrNoRows otherwise.
func madeKeyHash(ctx context.Context, q conn, id int64, label, actor string) (string, error) {
	var hash string
	err := q.QueryRowContext(ctx, "SELECT hash FROM api_keys WHERE id = ? AND label = ? AND created_by = ?", id, label, actor).Scan(&hash)
	return hash, err
}

// keyHashUsed reports whether any API key has this hash.
func keyHashUsed(ctx context.Context, q conn, hash string) (bool, error) {
	var used bool
	err := q.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM api_keys WHERE hash = ?)", hash).Scan(&used)
	return used, err
}

// setKeyScope stores an API key's scope list.
func setKeyScope(ctx context.Context, q conn, id int64, scope, now, actor string) error {
	_, err := q.ExecContext(ctx, "UPDATE api_keys SET scope = ?, updated_at = ?, updated_by = ? WHERE id = ?", scope, now, actor, id)
	return err
}
