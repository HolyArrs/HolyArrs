package ai

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// settingsGroup is the settings group the on/off switch lives in.
const settingsGroup = "this_app"

// enabled says whether AI connections are on; a missing row means off.
func enabled(ctx context.Context, st *database.Store) bool {
	return database.SettingOr(ctx, st.Read(), settingsGroup, settingMCP, "") == "on"
}

func saveEnabled(ctx context.Context, st *database.Store, who database.Actor, on bool) error {
	v := map[bool]string{true: "on", false: "off"}[on]
	return st.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, settingMCP, v, who, time.Now())
	})
}
