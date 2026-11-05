package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/auth"
)

// Only the owner can download the key kit, so only the owner is told to;
// others with Settings rights read that the owner should do it.
func TestKeyKitWarningNamesTheOwner(t *testing.T) {
	a, _ := signinRig(t, nil)
	exec41(t, a.s.Store, `INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed)
		VALUES ('backups:key', 'backups', 'Backups', 'warning', 'Download the key kit on the Backups page: backups do not hold the master key', '2026-11-01T00:00:00Z', '2026-11-01T00:00:00Z', 2, 'warning')`)
	msg := func(who auth.Who) string {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/health/checks", nil)
		r = r.WithContext(auth.ContextWith(r.Context(), who))
		a.Handler().ServeHTTP(w, r)
		var out struct {
			Checks []struct{ Key, Message string } `json:"checks"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		for _, c := range out.Checks {
			if c.Key == "backups:key" {
				return c.Message
			}
		}
		t.Fatalf("no backups:key in %s", w.Body.String())
		return ""
	}
	if m := msg(auth.Who{UserID: 1, Role: auth.RoleOwner}); !strings.HasPrefix(m, "Download the key kit") {
		t.Errorf("owner: %q", m)
	}
	admin := msg(auth.Who{UserID: 3, Role: "admin", Perms: []auth.Perm{auth.PermSettings}})
	if !strings.Contains(admin, "owner") || strings.HasPrefix(admin, "Download") {
		t.Errorf("admin: %q", admin)
	}
}
