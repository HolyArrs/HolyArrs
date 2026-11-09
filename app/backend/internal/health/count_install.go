package health

import (
	"context"
	"database/sql"
)

// SettingCountryCount is "Count this install" (the metadata server's
// per-country install total), in the this_app settings group.
const SettingCountryCount = "install_country_count"

// CountInstall reads the stored "Count this install" (on unless switched
// off). The app sets it on the metadata server's requests before any Job
// runs, so no request goes out counted after the owner switched it off.
func CountInstall(ctx context.Context, db *sql.DB) bool {
	var c string
	_ = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", SettingCountryCount).Scan(&c)
	return c != "off"
}
