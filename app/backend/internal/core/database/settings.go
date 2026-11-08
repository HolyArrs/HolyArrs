package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// SettingFound reads one value of the settings table and says whether the
// row exists.
func SettingFound(ctx context.Context, q Querier, grp, key string) (string, bool, error) {
	var v string
	err := q.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", grp, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

// Setting reads one value of the settings table; "" with no error when the
// row does not exist.
func Setting(ctx context.Context, q Querier, grp, key string) (string, error) {
	v, _, err := SettingFound(ctx, q, grp, key)
	return v, err
}

// SettingOr reads one value of the settings table; def when the row does not
// exist or cannot be read.
func SettingOr(ctx context.Context, q Querier, grp, key, def string) string {
	var v string
	if err := q.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", grp, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// EachSetting calls fn for every row of one settings group, in table order.
// Rows read before an error have already been passed to fn.
func EachSetting(ctx context.Context, q Lister, grp string, fn func(key, value string)) error {
	rows, err := q.QueryContext(ctx, "SELECT key, value FROM settings WHERE grp = ?", grp)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		fn(k, v)
	}
	return rows.Err()
}

// PutSetting writes one value, replacing a value already there.
func PutSetting(tx *sql.Tx, grp, key, value string, by Actor, now time.Time) error {
	who, _ := json.Marshal(by)
	t := FormatTime(now)
	_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		grp, key, value, t, string(who), t, string(who))
	return err
}

// PutSettingIfMissing writes one value unless the row already exists.
func PutSettingIfMissing(tx *sql.Tx, grp, key, value string, by Actor, now time.Time) error {
	who, _ := json.Marshal(by)
	t := FormatTime(now)
	_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(grp, key) DO NOTHING`,
		grp, key, value, t, string(who), t, string(who))
	return err
}

// SettingRow is one row of the settings table.
type SettingRow struct{ Grp, Key, Value string }

// Settings lists every row of the settings table.
func Settings(tx *sql.Tx) ([]SettingRow, error) {
	rows, err := tx.Query("SELECT grp, key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SettingRow
	for rows.Next() {
		var x SettingRow
		if err := rows.Scan(&x.Grp, &x.Key, &x.Value); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ReplaceSettingValue changes the value of a row that exists and leaves who
// changed it as it was.
func ReplaceSettingValue(tx *sql.Tx, grp, key, value string, now time.Time) error {
	_, err := tx.Exec("UPDATE settings SET value = ?, updated_at = ? WHERE grp = ? AND key = ?", value, FormatTime(now), grp, key)
	return err
}
