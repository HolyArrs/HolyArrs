package health

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// metadataModeChecks says where details come from and returns true when the
// metadata server is not to be asked: in Your own keys and Off mode the
// doctor never contacts it either. The Docker setting wins over the one
// picked in Settings (read from db when there is one).
func metadataModeChecks(ctx context.Context, r *Report, db database.Lister) bool {
	const s = "Network"
	m := metadata.EnvModes()
	if bad := m.BadValue(); bad != "" {
		r.add(s, StatusFail, metadata.EnvMode+" is "+quote(bad)+", which is not full, own or off, so lookups are off",
			"set "+metadata.EnvMode+" to full, own or off, or remove it")
		return true
	}
	mode := m.Current()
	if !m.Locked() && db != nil {
		_ = database.EachSetting(ctx, db, metadata.ModeGroup, func(key, value string) {
			if key == metadata.ModeKey {
				if v, ok := metadata.ParseMode(value); ok {
					mode = v
				}
			}
		})
	}
	switch mode {
	case metadata.ModeOwn:
		r.add(s, StatusOK, "metadata server: not asked (details come from your own keys)", "")
		return true
	case metadata.ModeOff:
		r.add(s, StatusOK, "metadata server: not asked (lookups are off on this install)", "")
		return true
	}
	return false
}

func quote(v string) string {
	if len(v) > 40 {
		v = v[:40] + "..."
	}
	return "\"" + v + "\""
}
