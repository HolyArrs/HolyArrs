package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The settings objects the apps send each side (delay rules, size table,
// media switches, naming) are kept as sent in the this_app group, one key
// per side and kind ("old_app_api_radarr_delay").
func configKey(side, kind string) string { return settingOn + "_" + side + "_" + kind }

// readConfig reads one stored object into v. found is false when the app
// never sent one; a broken value reads as never sent.
func readConfig(ctx context.Context, st *database.Store, side, kind string, v any) (bool, error) {
	s, found, err := database.SettingFound(ctx, st.Read(), settingsGroup, configKey(side, kind))
	if err != nil || !found {
		return false, err
	}
	return json.Unmarshal([]byte(s), v) == nil, nil
}

// saveConfig stores one object, replacing what was there.
func saveConfig(ctx context.Context, st *database.Store, side, kind string, v any, by domain.Actor) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return st.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, configKey(side, kind), string(b), by, time.Now())
	})
}
