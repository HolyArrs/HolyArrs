package metadata

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// StoredMode reads the mode picked in Settings; ok is false when none was
// picked (or the row holds something unknown), which means the metadata
// server.
func StoredMode(ctx context.Context, q database.Querier) (Mode, bool) {
	v, found, err := database.SettingFound(ctx, q, ModeGroup, ModeKey)
	if err != nil || !found {
		return ModeServer, false
	}
	return ParseMode(v)
}

// SaveMode stores the mode picked in Settings.
func SaveMode(ctx context.Context, st *database.Store, m Mode, by database.Actor) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, ModeGroup, ModeKey, string(m), by, time.Now())
	})
}

// LoadMode applies the stored mode at start; the Docker setting, when there
// is one, wins.
func (m *Modes) LoadMode(ctx context.Context, q database.Querier) {
	if m.Locked() {
		return
	}
	if v, ok := StoredMode(ctx, q); ok {
		_ = m.Set(v)
	}
}
