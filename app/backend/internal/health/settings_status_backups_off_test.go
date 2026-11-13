package health

import (
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// With backups turned off, an old or missing backup and a left-over
// backups check raise nothing; the Backups page still counts as known.
func TestBackupsOffRaisesNoStatus(t *testing.T) {
	for _, newest := range []string{"", "2026-10-01T00:00:00Z"} {
		a := SettingsStatus(StatusInput{NewestBackup: timep(newest), BackupsOff: true, Now: statusNow,
			Checks: append([]domain.HealthCheck(nil), check("backups", "Backups", Warning, "There is no backup yet"))})
		if len(a.Items) != 0 {
			t.Fatalf("newest %q: %+v", newest, a.Items)
		}
		if !slices.Contains(a.Known, "/settings/backups") {
			t.Fatalf("newest %q: known %v", newest, a.Known)
		}
	}
	a := SettingsStatus(StatusInput{NewestBackup: timep(""), Now: statusNow})
	if len(a.Items) != 1 {
		t.Fatalf("backups on: %+v", a.Items)
	}
}
