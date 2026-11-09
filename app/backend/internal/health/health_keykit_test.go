package health

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Only the owner can download the key kit, so only the owner is told to;
// others with Settings rights read that the owner should do it.
func TestKeyKitWarningNamesTheOwner(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	mux := newRig(t, s, rigOpts{}).mux
	execSQL(t, s, `INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed)
		VALUES ('backups:key', 'backups', 'Backups', 'warning', 'Download your key kit: Backups don''t include the master key', '2026-11-01T00:00:00Z', '2026-11-01T00:00:00Z', 2, 'warning')`)
	msg := func(who webserver.Caller) string {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/health/checks", nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), who))
		mux.ServeHTTP(w, r)
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
	if m := msg(webserver.Caller{UserID: 1, Role: webserver.RoleOwner}); !strings.HasPrefix(m, "Download your key kit") {
		t.Errorf("owner: %q", m)
	}
	admin := msg(webserver.Caller{UserID: 3, Role: "admin", Perms: []webserver.Perm{webserver.PermSettings}})
	if !strings.Contains(admin, "owner") || strings.HasPrefix(admin, "Download") {
		t.Errorf("admin: %q", admin)
	}
}

// Settings home reads the same warning from a cache shared by everyone, so
// it is reworded per caller after the cache, without changing the cache.
func TestKeyKitStatusNamesTheOwner(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	mux := newRig(t, s, rigOpts{}).mux
	execSQL(t, s, `INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed)
		VALUES ('backups:key', 'backups', 'Backups', 'warning', 'Download your key kit: Backups don''t include the master key', '2026-11-01T00:00:00Z', '2026-11-01T00:00:00Z', 2, 'warning')`)
	text := func(who webserver.Caller) string {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/settings/status", nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), who))
		mux.ServeHTTP(w, r)
		var out struct {
			Items []struct{ Text string } `json:"items"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		for _, i := range out.Items {
			if strings.Contains(i.Text, "key kit") {
				return i.Text
			}
		}
		t.Fatalf("no key kit item in %d %s", w.Code, w.Body.String())
		return ""
	}
	owner := webserver.Caller{UserID: 1, Role: webserver.RoleOwner}
	if m := text(owner); !strings.Contains(m, "Download your key kit") {
		t.Errorf("owner: %q", m)
	}
	if m := text(webserver.Caller{UserID: 3, Role: "admin", Perms: []webserver.Perm{webserver.PermSettings}}); !strings.Contains(m, "Ask the owner") || strings.Contains(m, "Download your key kit") {
		t.Errorf("admin: %q", m)
	}
	if m := text(owner); !strings.Contains(m, "Download your key kit") {
		t.Errorf("owner after admin: %q", m)
	}
}
