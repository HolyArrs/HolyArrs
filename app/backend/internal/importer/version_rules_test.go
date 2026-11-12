package importer

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A 1080p file tied to a 4K Version must never replace the 4K copy, even when the copy in place has no quality in its name.
func TestStepNotAllowedNeverReplacesCopy(t *testing.T) {
	for _, old := range []string{"Dune - Part Two (2024)", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-HOLYARRS"} {
		w, relID, oldID := upgradeWorld(t, relName, relName, old)
		ctx := context.Background()
		if err := w.s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE versions SET pick_rules_id = (SELECT id FROM pick_rules WHERE name = ?)", mediafiles.FourKName)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if imp.State == "done" {
			t.Fatalf("%s: a 1080p file replaced the 4K copy", old)
		}
		var f mediafiles.File
		_ = database.Get(ctx, w.s.Read(), "files", oldID, &f)
		if sl := slotOf(t, w); f.State != "present" || sl.FileID == nil || *sl.FileID != oldID {
			t.Fatalf("%s: old file %s, slot file %v", old, f.State, sl.FileID)
		}
		if old == "Dune - Part Two (2024)" {
			var rel domain.Release
			_ = database.Get(ctx, w.s.Read(), "releases", relID, &rel)
			why, _ := rel.Verdict["turned_down_reason"].(string)
			if !strings.Contains(why, "Pick rules") || !strings.Contains(why, "another Version") {
				t.Fatalf("reason %q", why)
			}
		}
	}
}
