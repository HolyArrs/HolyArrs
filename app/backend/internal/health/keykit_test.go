package health

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// The "backups:key" result follows the backups module's reminder level:
// nothing on a fresh install, a calm OK note with a message once there is
// something to protect, and a Warning only at real risk. The ordinary
// backups result is not changed by it.
func TestBackupsKeyKitCheck(t *testing.T) {
	w := newWorld(t)
	if c, ok := w.run(t)["backups:key"]; ok && (c.State != OK || c.Message != "") {
		t.Fatalf("fresh install: %+v, want no reminder", c)
	}
	w.bk.KeyKit = secrets.KitRecommend
	got := w.run(t)
	if c := got["backups:key"]; c.State != OK || c.Message != KeyKitRecommended {
		t.Fatalf("first secret: %+v, want a calm OK note", c)
	}
	w.bk.KeyKit = secrets.KitWarn
	got = w.run(t)
	if c := got["backups:key"]; c.State != Warning || c.Kind != "backups" || c.Message != KeyKitWarning {
		t.Fatalf("at risk: %+v, want a backups warning", c)
	}
	if b := got["backups"]; b.State != OK {
		t.Fatalf("the key warning changed the backups result: %+v", b)
	}
	w.bk.KeyKit = ""
	if c := w.run(t)["backups:key"]; c.State != OK || c.Message != "" {
		t.Fatalf("kit saved: %+v, want cleared", c)
	}
}
