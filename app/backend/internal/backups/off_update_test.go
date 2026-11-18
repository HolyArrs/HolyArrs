package backups

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// With Off, an update this start made and checked leaves no backup behind:
// no update backup, and the copy from before the data update is removed.
// A backup made by hand still works.
func TestOffMakesNoBackupOnUpdate(t *testing.T) {
	dir := t.TempDir()
	ms, err := database.EmbeddedMigrations("migrations/main")
	if err != nil || len(ms) < 2 {
		t.Fatal(err, len(ms))
	}
	old := fstest.MapFS{}
	for _, m := range ms[:len(ms)-1] {
		old[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
	}
	s, err := database.Open(dir, database.Options{MainMigrations: old})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: s, Dir: dir, Version: "1.2.3"}
	ctx := t.Context()
	if err := m.AfterStart(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPreset(ctx, Off, domain.Actor{}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if s, err = database.Open(dir, database.Options{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if len(s.CheckedUpdateCopies()) != 1 {
		t.Fatalf("checked copies %v, want the main one", s.CheckedUpdateCopies())
	}
	m = &Manager{Store: s, Dir: dir, Version: "1.2.4"}
	if _, err := m.AdoptMigrationCopies(); err != nil {
		t.Fatal(err)
	}
	if err := m.AfterStart(ctx); err != nil {
		t.Fatal(err)
	}
	if all, _ := m.List(); len(all) != 0 {
		t.Fatalf("backups made with Off on an update: %+v", all)
	}
	if raws, _ := filepath.Glob(filepath.Join(dir, "backups", "*.db")); len(raws) != 0 {
		t.Fatalf("copy from before the update still there: %v", raws)
	}
	if _, err := m.Create(ctx, KindManual); err != nil {
		t.Fatalf("manual backup with Off: %v", err)
	}
}

// A copy this start did not make and check (an update cut off, or one whose
// check failed, then a restart with nothing left to update) is the only way
// back, so with Off it is still kept as a pre-update backup.
func TestOffKeepsACopyNotCheckedThisStart(t *testing.T) {
	m, s, _, _ := setup(t)
	ctx := t.Context()
	if err := m.SetPreset(ctx, Off, domain.Actor{}); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(m.backupDir(), 0o750)
	src, _, err := s.Snapshot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(src)
	raw := filepath.Join(m.backupDir(), "main-pre-update-0001-to-0002-20261023T030000.000Z.db")
	_ = os.WriteFile(raw, b, 0o600)
	if n, err := m.AdoptMigrationCopies(); err != nil || n != 1 {
		t.Fatalf("adopted %d, %v", n, err)
	}
	all, _ := m.List()
	if len(all) != 1 || all[0].Kind != KindPreUpdate {
		t.Fatalf("leftover copy not kept: %+v", all)
	}
}
