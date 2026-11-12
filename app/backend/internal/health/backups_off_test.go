package health

import (
	"testing"
	"time"
)

// With backups off, neither a missing or old backup nor a missing key kit
// is a warning.
func TestBackupsOffNoWarnings(t *testing.T) {
	w := newWorld(t)
	w.bk = Backups{Off: true, Started: true, KeyPrint: "abcd"}
	got := w.run(t)
	if c := got["backups"]; c.State != OK {
		t.Fatalf("no backup with Off %+v", c)
	}
	if c, ok := got["backups:key"]; ok && c.State != OK {
		t.Fatalf("key kit with Off %+v", c)
	}
	w.bk.Newest = w.now.Add(-60 * 24 * time.Hour)
	if c := w.run(t)["backups"]; c.State != OK {
		t.Fatalf("old backup with Off %+v", c)
	}
}
