package backup

import (
	"context"
	"strings"
	"testing"
)

// A scheduled backup that clearly cannot fit on the config disk is skipped
// with a plain message, so it never fills the disk the database needs.
func TestScheduledBackupSkippedWhenTheDiskIsFull(t *testing.T) {
	m, _, _, _ := setup(t)
	m.FreeSpace = func(string) int64 { return 1024 }
	err := m.Pass(context.Background())
	if err == nil {
		t.Fatal("want the backup skipped")
	}
	_, _, last, _ := m.State(context.Background())
	for _, want := range []string{"skipped", "free"} {
		if !strings.Contains(last, want) {
			t.Errorf("message lacks %q: %s", want, last)
		}
	}
	if all, _ := m.List(); len(all) != 0 {
		t.Fatalf("no backup should be made: %v", all)
	}
	// Unknown free space (-1) and plenty of room both make the backup.
	for _, free := range []int64{-1, 50 << 30} {
		m.FreeSpace = func(string) int64 { return free }
		if err := m.Pass(context.Background()); err != nil {
			t.Fatalf("free %d: %v", free, err)
		}
		all, _ := m.List()
		if len(all) == 0 {
			t.Fatalf("free %d: no backup", free)
		}
		for _, b := range all {
			_ = m.Delete(b.Name)
		}
	}
}
