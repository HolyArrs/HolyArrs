package health

import "testing"

// The Backups card's key warning also goes through the health engine: a
// "backups:key" result warns until a key kit matching the current key has
// been downloaded. No current fingerprint (HOLYARRS_MASTER_KEY_FILE is
// set) skips it. The ordinary backups result is not changed by it.
func TestBackupsKeyKitCheck(t *testing.T) {
	w := newWorld(t)
	w.bk.KeyPrint = "1a2b3c4d"
	got := w.run(t)
	c, ok := got["backups:key"]
	if !ok {
		t.Fatalf("no backups:key result in %+v", got)
	}
	if c.State != Warning || c.Kind != "backups" || c.Message == "" {
		t.Fatalf("no kit yet: %+v, want a backups warning", c)
	}
	if b := got["backups"]; b.State != OK {
		t.Fatalf("the key warning changed the backups result: %+v", b)
	}
	w.bk.KeyKitPrint = "ffff0000"
	if c := w.run(t)["backups:key"]; c.State != Warning {
		t.Fatalf("kit for another key: %+v, want warning", c)
	}
	w.bk.KeyKitPrint = "1a2b3c4d"
	if c := w.run(t)["backups:key"]; c.State != OK {
		t.Fatalf("matching kit: %+v, want ok", c)
	}
	w.bk.KeyPrint, w.bk.KeyKitPrint = "", ""
	if c, ok := w.run(t)["backups:key"]; ok && c.State != OK {
		t.Fatalf("named key file: the key check should be skipped, got %+v", c)
	}
}
