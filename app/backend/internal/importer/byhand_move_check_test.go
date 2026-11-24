package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Move removes the original only once the Library file is checked: the same
// file (a hardlink) or a copy of the same size. A short copy keeps it.
func TestByHandMoveKeepsSourceUntilChecked(t *testing.T) {
	exdev := func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} }
	short := func(src, dst string) error { return os.WriteFile(dst, []byte("cut"), 0o644) }
	// The source is written to while it is copied: its time moves past the start.
	changed := func(src, dst string) error {
		if err := copyFile(src, dst); err != nil {
			return err
		}
		later := time.Now().Add(time.Hour)
		return os.Chtimes(src, later, later)
	}
	cases := []struct {
		name     string
		ops      Ops
		mode     string
		kept     bool
		placedBy string
		warn     bool
	}{
		{name: "hardlink move", mode: "move", placedBy: "move"},
		{name: "copy move", ops: Ops{Link: exdev}, mode: "move", placedBy: "copy"},
		{name: "short copy", ops: Ops{Link: exdev, Copy: short}, mode: "move", kept: true, placedBy: "copy", warn: true},
		{name: "changed while copied", ops: Ops{Link: exdev, Copy: changed}, mode: "move", kept: true, placedBy: "copy", warn: true},
		{name: "copy mode", mode: "copy", kept: true, placedBy: "hardlink"},
		{name: "default mode", kept: true, placedBy: "hardlink"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := setup(t)
			ctx := context.Background()
			if err := w.s.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec("UPDATE slots SET state = 'wanted' WHERE id = ?", w.slotID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			loose := t.TempDir()
			src := filepath.Join(loose, "Dune.Part.Two.2024.2160p.WEB-DL.mkv")
			write(t, src, 4096)
			im := &Importer{Store: w.s, SavePath: loose, Ops: c.ops}
			res, err := im.ImportByHand(ctx, []ByHandItem{{Path: src, SlotID: w.slotID, Mode: c.mode}}, domain.System)
			if err != nil || len(res) != 1 || res[0].Outcome != "placed" {
				t.Fatalf("import %+v %v", res, err)
			}
			if res[0].PlacedBy != c.placedBy {
				t.Fatalf("placed by %q, want %q", res[0].PlacedBy, c.placedBy)
			}
			_, serr := os.Stat(src)
			if (serr == nil) != c.kept {
				t.Fatalf("source kept %v, want %v", serr == nil, c.kept)
			}
			if c.warn != strings.Contains(res[0].Error, "original") {
				t.Fatalf("note %q", res[0].Error)
			}
		})
	}
}
