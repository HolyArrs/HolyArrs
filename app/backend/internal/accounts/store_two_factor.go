package accounts

import (
	"context"
	"database/sql"
)

// twoFactorOn says whether a person has two-factor sign-in on.
func twoFactorOn(ctx context.Context, q conn, userID int64) (bool, error) {
	var on bool
	err := q.QueryRowContext(ctx, "SELECT totp_secret_id IS NOT NULL FROM users WHERE id = ?", userID).Scan(&on)
	return on, err
}

// totpState is the secret id and last used step of a person's authenticator.
func totpState(ctx context.Context, q conn, userID int64) (sid sql.NullInt64, last int64, err error) {
	err = q.QueryRowContext(ctx, "SELECT totp_secret_id, totp_last_step FROM users WHERE id = ?", userID).Scan(&sid, &last)
	return sid, last, err
}

// useTotpStep records a code step as used; it returns how many rows it
// changed, so a step used twice changes none.
func useTotpStep(ctx context.Context, q conn, step, userID int64) (int64, error) {
	res, err := q.ExecContext(ctx, "UPDATE users SET totp_last_step = ? WHERE id = ? AND totp_last_step < ?", step, userID, step)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// useBackupCode marks a backup code used and returns how many rows it
// changed.
func useBackupCode(ctx context.Context, q conn, at string, userID int64, hash string) (int64, error) {
	res, err := q.ExecContext(ctx, "UPDATE backup_codes SET used_at = ? WHERE user_id = ? AND hash = ? AND used_at IS NULL",
		at, userID, hash)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// pendingTotpID is the secret id of a setup that is not confirmed yet.
func pendingTotpID(ctx context.Context, q conn, userID int64) (sql.NullInt64, error) {
	var id sql.NullInt64
	err := q.QueryRowContext(ctx, "SELECT totp_pending_id FROM users WHERE id = ?", userID).Scan(&id)
	return id, err
}

// setPendingTotp keeps the secret of a setup that is not confirmed yet.
func setPendingTotp(ctx context.Context, q conn, secretID, userID int64) error {
	_, err := q.ExecContext(ctx, "UPDATE users SET totp_pending_id = ? WHERE id = ?", secretID, userID)
	return err
}

// confirmTotp turns the pending secret into the person's authenticator.
func confirmTotp(ctx context.Context, q conn, step, userID int64) error {
	_, err := q.ExecContext(ctx, "UPDATE users SET totp_secret_id = totp_pending_id, totp_pending_id = NULL, totp_last_step = ? WHERE id = ?", step, userID)
	return err
}

// totpSecretIDs lists the secret ids of a person's authenticator and of a
// setup that is not confirmed yet.
func totpSecretIDs(ctx context.Context, q conn, userID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx, "SELECT totp_secret_id FROM users WHERE id = ? UNION SELECT totp_pending_id FROM users WHERE id = ?", userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id.Valid {
			ids = append(ids, id.Int64)
		}
	}
	return ids, rows.Err()
}

// resetTotpColumns clears a person's authenticator columns.
func resetTotpColumns(ctx context.Context, q conn, userID int64) error {
	_, err := q.ExecContext(ctx, "UPDATE users SET totp_secret_id = NULL, totp_pending_id = NULL, totp_last_step = 0 WHERE id = ?", userID)
	return err
}

// deleteBackupCodes removes a person's backup codes.
func deleteBackupCodes(ctx context.Context, q conn, userID int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM backup_codes WHERE user_id = ?", userID)
	return err
}

// insertBackupCode stores the hash of one backup code.
func insertBackupCode(ctx context.Context, q conn, userID int64, hash string) error {
	_, err := q.ExecContext(ctx, "INSERT INTO backup_codes(user_id, hash) VALUES (?, ?)", userID, hash)
	return err
}
