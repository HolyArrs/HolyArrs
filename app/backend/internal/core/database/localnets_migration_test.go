package database

import (
	"io/fs"
	"strings"
	"testing"
)

// TestLocalNetworksMigration: an install that skipped sign-in on the local
// network keeps the private ranges it trusted before; one that did not gets
// no list.
func TestLocalNetworksMigration(t *testing.T) {
	src, _ := fs.Sub(embedded, "migrations/main")
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	v := 0
	for _, m := range ms {
		if strings.HasSuffix(m.name, "_local_networks.sql") {
			v = m.version
		}
	}
	if v == 0 {
		t.Fatal("local networks migration missing")
	}
	for skip, want := range map[string]bool{"true": true, "false": false} {
		dir := t.TempDir()
		s, err := Open(dir, Options{MainMigrations: upTo(t, v-1)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.main.db.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('this_app', 'skip_login_local_network', ?, '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`, skip); err != nil {
			t.Fatal(err)
		}
		_ = s.Close()
		s, err = Open(dir, Options{})
		if err != nil {
			t.Fatal(err)
		}
		var nets string
		err = s.Read().QueryRow("SELECT value FROM settings WHERE grp = 'this_app' AND key = 'sign_in_local_networks'").Scan(&nets)
		_ = s.Close()
		if got := err == nil; got != want {
			t.Fatalf("skip %s: list stored %t (%v)", skip, got, err)
		}
		if want && (!strings.Contains(nets, `"192.168.0.0/16"`) || !strings.Contains(nets, `"fc00::/7"`) || !strings.Contains(nets, `"127.0.0.0/8"`)) {
			t.Fatalf("skip %s: list %s", skip, nets)
		}
	}
}
