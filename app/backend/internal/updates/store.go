package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// setting reads one update check value from the this_app group; "" when unset.
func (e *Checker) setting(ctx context.Context, key string) string {
	var v string
	_ = e.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
	return v
}

// intSetting reads a whole number, falling back to def outside lo..hi.
func (e *Checker) intSetting(ctx context.Context, key string, def, lo, hi int) int {
	n, err := strconv.Atoi(e.setting(ctx, key))
	if err != nil || n < lo || n > hi {
		return def
	}
	return n
}

// saveSetting keeps one update check value in the this_app group.
func (e *Checker) saveSetting(ctx context.Context, key, value string) error {
	now := database.FormatTime(time.Now())
	by, _ := json.Marshal(domain.System)
	return e.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			key, value, now, string(by), now, string(by))
		return err
	})
}
