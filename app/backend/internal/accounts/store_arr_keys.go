package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// arrKeyOwner finds the active person behind a side's key hash. The person
// comes back with the key's last-used time and their stored permissions.
func (a *Auth) arrKeyOwner(ctx context.Context, side, hash string) (who webserver.Caller, used sql.NullString, perms string, err error) {
	err = a.s.Read().QueryRowContext(ctx, `SELECT k.last_used_at, `+userCols+` FROM compat_keys k JOIN users u ON u.id = k.owner_user_id
		WHERE k.side = ? AND k.hash = ? AND u.state = 'active'`, side, hash).Scan(&used, &who.UserID, &who.Name, &who.Role, &perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry)
	return who, used, perms, err
}

func (a *Auth) touchArrKey(ctx context.Context, side string, now time.Time) error {
	return a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE compat_keys SET last_used_at = ? WHERE side = ?", database.FormatTime(now), side)
		return err
	})
}

func (a *Auth) listArrKeys(ctx context.Context) ([]ArrKeyView, error) {
	rows, err := a.s.Read().QueryContext(ctx, `SELECT k.side, k.hint, COALESCE(u.name, ''), COALESCE(u.state = 'active', 0), k.created_at, k.last_used_at
		FROM compat_keys k LEFT JOIN users u ON u.id = k.owner_user_id ORDER BY k.side DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ArrKeyView{}
	for rows.Next() {
		var v ArrKeyView
		var used sql.NullString
		if err := rows.Scan(&v.Side, &v.Hint, &v.Owner, &v.OwnerOK, &v.CreatedAt, &used); err != nil {
			return nil, err
		}
		v.Hint = "\u2026" + v.Hint
		if used.Valid {
			v.LastUsedAt = &used.String
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// putArrKey stores a side's key hash and last four characters, replacing an
// older key of that side.
func (a *Auth) putArrKey(ctx context.Context, side string, ownerID int64, k string) error {
	by, _ := json.Marshal(webserver.Actor(ctx))
	now := database.FormatTime(a.now())
	return a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO compat_keys(side, created_at, created_by, owner_user_id, hash, hint) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (side) DO UPDATE SET created_at = excluded.created_at, created_by = excluded.created_by, owner_user_id = excluded.owner_user_id,
			last_used_at = NULL, hash = excluded.hash, hint = excluded.hint`, side, now, string(by), ownerID, sum(k), k[len(k)-4:])
		return err
	})
}

// deleteArrKeysOf removes the arr API keys a person made. It returns how many went.
func deleteArrKeysOf(ctx context.Context, tx *sql.Tx, userID int64) (int64, error) {
	res, err := tx.ExecContext(ctx, "DELETE FROM compat_keys WHERE owner_user_id = ?", userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// removeArrKey says how many keys it removed.
func (a *Auth) removeArrKey(ctx context.Context, side string) (int64, error) {
	var n int64
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("DELETE FROM compat_keys WHERE side = ?", side)
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	return n, err
}
