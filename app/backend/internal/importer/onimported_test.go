package importer

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// A done Import tells OnImported once; a dry run, a rejected Import and a
// second refused run do not.
func TestOnImportedOnlyAfterDone(t *testing.T) {
	ctx := context.Background()
	var told []int64
	hook := func(_ context.Context, id int64) { told = append(told, id) }

	w := setup(t)
	im := &Importer{Store: w.s, OnImported: hook}
	if _, err := im.Import(ctx, w.dlID, true, domain.System); err != nil || len(told) != 0 {
		t.Fatalf("dry run told %v (%v)", told, err)
	}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("import %v %v", imp, err)
	}
	if len(told) != 1 || told[0] != w.dlID {
		t.Fatalf("done import told %v", told)
	}

	// A rejected Import (the file lied about its quality) leaves the job alone.
	told = nil
	lw, _, _ := upgradeWorld(t, "Dune.Part.Two.2024.720p.WEB.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "")
	im = &Importer{Store: lw.s, OnRejected: func(context.Context, int64) {}, OnImported: hook}
	if imp, err := im.Import(ctx, lw.dlID, false, domain.System); err != nil || imp.State != "rejected" {
		t.Fatalf("import %v %v", imp, err)
	}
	if len(told) != 0 {
		t.Fatalf("rejected import told %v", told)
	}
}
