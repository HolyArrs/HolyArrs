package updates

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// settingsGroup holds the update check's values.
const settingsGroup = "this_app"

// setting reads one update check value; "" when unset or unreadable.
func (e *Checker) setting(ctx context.Context, key string) string {
	return database.SettingOr(ctx, e.Store.Read(), settingsGroup, key, "")
}

// intSetting reads a whole number, falling back to def outside lo..hi.
func (e *Checker) intSetting(ctx context.Context, key string, def, lo, hi int) int {
	n, err := strconv.Atoi(e.setting(ctx, key))
	if err != nil || n < lo || n > hi {
		return def
	}
	return n
}

// saveSetting keeps one update check value.
func (e *Checker) saveSetting(ctx context.Context, key, value string) error {
	return e.Store.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, key, value, domain.System, time.Now())
	})
}
