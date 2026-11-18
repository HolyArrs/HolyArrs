package updates_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A release that keeps the database must not drop the copy an earlier,
// changing release made: going back past it still needs that copy.
func TestNonMigratingInstallKeepsEarlierCopy(t *testing.T) {
	v := newVolume(t)
	n := 0
	in := v.installer(&n)
	in.Snapshot = func(dir string) error {
		return os.WriteFile(filepath.Join(dir, database.MainFile), []byte("1.1.4 data"), 0o600)
	}
	r2 := newRelease(t, v.key, "program 1.2.0")
	c2 := r2.channel("1.2.0")
	c2.Migrates = true
	if err := in.Prepare(context.Background(), emptyList, nil, nil, c2); err != nil {
		t.Fatal(err)
	}
	if err := in.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 1.2.0 now runs and installs 1.3.0, which keeps the database.
	in = v.installer(&n)
	in.Version = "1.2.0"
	in.Snapshot = func(string) error { t.Fatal("copied for a release that keeps the database"); return nil }
	r3 := newRelease(t, v.key, "program 1.3.0")
	if err := in.Prepare(context.Background(), emptyList, nil, nil, r3.channel("1.3.0")); err != nil {
		t.Fatal(err)
	}
	if err := in.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !v.exists(filepath.Join("dbcopy", database.MainFile)) {
		t.Fatal("the 1.1.4 copy was removed")
	}
	writeFile(t, filepath.Join(v.cfg, database.MainFile), "1.2.0 data")
	// 1.3.0 fails and goes back to 1.2.0: the copy belongs to 1.2.0, so
	// the database stays as it is and the copy is kept.
	l := v.launcher()
	for range 3 {
		if cur, ok := l.Pick(); ok && cur.Version == "1.2.0" {
			break
		}
	}
	if b, _ := os.ReadFile(filepath.Join(v.cfg, database.MainFile)); string(b) != "1.2.0 data" {
		t.Fatalf("going back to 1.2.0 put back %q", b)
	}
	if !v.exists(filepath.Join("dbcopy", database.MainFile)) {
		t.Fatal("copy lost going back to 1.2.0")
	}
}

// With no copy for a release that changed the database, the image's older
// version is not offered as a go-back target.
func TestImageNotOfferedWithoutCopyAfterMigration(t *testing.T) {
	t.Setenv("HOLYARRS_FROM_VOLUME", "1")
	t.Setenv("HOLYARRS_IMAGE_VERSION", "1.1.4")
	v := newVolume(t)
	cur := v.install("1.2.0", "program")
	cur.Migrates = true
	v.set("current.json", cur)
	n := 0
	in := v.installer(&n)
	in.Version = "1.2.0"
	if got := in.Previous("1.2.0"); got != "" {
		t.Fatalf("offered %q with no database copy", got)
	}
	if err := in.GoBack(); err == nil || n != 0 {
		t.Fatal("went back onto a changed database with no copy")
	}
	writeFile(t, v.app(filepath.Join("dbcopy", database.MainFile)), "before")
	writeFile(t, v.app(filepath.Join("dbcopy", "for")), "1.2.0")
	if got := in.Previous("1.2.0"); got != "1.1.4" {
		t.Fatalf("previous %q with a copy", got)
	}
}
