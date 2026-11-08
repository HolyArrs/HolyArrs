package arrapi

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// switchValue reads one of the two switches; "" when it was never saved.
func switchValue(ctx context.Context, st *database.Store, key string) string {
	v, _ := database.Setting(ctx, st.Read(), settingsGroup, key)
	return v
}

// saveSwitches writes the switches that are not nil, in one transaction.
func saveSwitches(ctx context.Context, st *database.Store, who domain.Actor, values map[string]*bool) error {
	now := time.Now()
	return st.Write(ctx, func(tx *sql.Tx) error {
		for key, v := range values {
			if v == nil {
				continue
			}
			if err := database.PutSetting(tx, settingsGroup, key, map[bool]string{true: "on", false: "off"}[*v], who, now); err != nil {
				return err
			}
		}
		return nil
	})
}
