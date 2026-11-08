package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestProtectedCopyCannotBeReplaced(t *testing.T) {
	w, _, oldID := upgradeWorld(t, "Dune.Part.Two.2024.1080p.BluRay.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.BluRay.H264-HOLYARRS", "Dune.Part.Two.2024.720p.WEB.H264-HOLYARRS")
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET protected = 1 WHERE id = ?", oldID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var old mediafiles.File
	if err := database.Get(ctx, w.s.Read(), "files", oldID, &old); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(w.lib, filepath.FromSlash(old.RelativePath))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	p, err := im.Import(ctx, w.dlID, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if hasMain(p.Plan) {
		t.Fatal("preview would replace protected copy")
	}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" {
		t.Fatalf("state %s", imp.State)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("protected bytes changed: %v", err)
	}
	if sl := slotOf(t, w); sl.State != mediafiles.SlotNeedsADecision || sl.FileID == nil || *sl.FileID != oldID {
		t.Fatalf("protected slot replaced: %+v", sl)
	}
}
