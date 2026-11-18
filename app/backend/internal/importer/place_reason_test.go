package importer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A movie whose file matched but could not be placed says why, not that no file matched.
func TestMovieImportSaysWhyThePlacingFailed(t *testing.T) {
	for _, c := range []struct {
		name  string
		ops   Ops
		other bool
		dir   bool
		want  string
	}{
		{name: "a different file is already at the target", other: true, want: targetTakenWhy},
		{name: "a folder is already at the target", other: true, dir: true, want: targetTakenWhy},
		{name: "the folder is not writable", ops: Ops{
			Link: func(_, _ string) error { return &os.LinkError{Op: "link", Err: fs.ErrPermission} },
			Copy: func(_, dst string) error {
				return &fs.PathError{Op: "open", Path: filepath.Join(filepath.Dir(dst), ".holyarrs-1.part"), Err: fs.ErrPermission}
			},
		}, want: noWriteWhy},
		{name: "the downloaded file is not readable", ops: Ops{
			Link: func(_, _ string) error { return &os.LinkError{Op: "link", Err: fs.ErrPermission} },
			Copy: func(src, _ string) error { return &fs.PathError{Op: "open", Path: src, Err: fs.ErrPermission} },
		}, want: noReadWhy},
		{name: "something else failed", ops: Ops{
			Link: func(_, _ string) error { return errors.New("link refused") },
			Copy: func(_, _ string) error { return errors.New("disk exploded") },
		}, want: "the file could not be placed: hardlink failed (link refused), copy failed (disk exploded)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := setup(t)
			ctx := context.Background()
			if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
				t.Fatal(err)
			}
			im := &Importer{Store: w.s, Ops: c.ops}
			if c.other {
				dry, err := im.Import(ctx, w.dlID, true, domain.System)
				if err != nil || len(dry.Plan) != 1 || dry.Plan[0].Method != "hardlink" {
					t.Fatalf("plan %+v %v", dry, err)
				}
				if c.dir {
					if err := os.MkdirAll(dry.Plan[0].TargetPath, 0o755); err != nil {
						t.Fatal(err)
					}
				} else {
					write(t, dry.Plan[0].TargetPath, 4096)
				}
			}
			imp, err := im.Import(ctx, w.dlID, false, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			if imp.State != "failed" {
				t.Fatalf("import %s %+v", imp.State, imp.Result)
			}
			var sl mediafiles.Slot
			if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil {
				t.Fatal(err)
			}
			if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != c.want || strings.Contains(*sl.LastReason, "matched") {
				t.Fatalf("slot %s reason %v, want %q", sl.State, ptrText(sl.LastReason), c.want)
			}
		})
	}
}

func ptrText(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Each plain line names what was refused: the file and its folder for a read, the folder the file goes to for a write.
func TestPlainLinesNameWhatWasRefused(t *testing.T) {
	if !strings.Contains(noReadWhy, "read access to the file and the folder it is in") {
		t.Fatal(noReadWhy)
	}
	if !strings.Contains(noWriteWhy, "write to the folder this file goes to in the Library") {
		t.Fatal(noWriteWhy)
	}
}

// Import this file that cannot be placed leaves why on the movie each time, so Review never keeps the line of an earlier try.
func TestReviewImportThatCannotBePlacedSaysWhyOnTheMovie(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	setFilesSeen(t, w, "Some.Other.Name.2016.German.DL.1080p.BluRay.x264-GROUP/group-sn.mkv")
	im := &Importer{Store: w.s}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State == "done" {
		t.Fatalf("auto import should not place it: %+v %v", imp, err)
	}
	old := ptrText(slot308(t, w).LastReason)
	if err := execSQL(w.s, "UPDATE slots SET updated_at = '2020-01-02T03:04:05.000000000Z' WHERE id = ?", w.slotID); err != nil {
		t.Fatal(err)
	}
	since := slotSince(t, w)
	im.Ops = Ops{
		Link: func(_, _ string) error { return errors.New("link refused") },
		Copy: func(_, _ string) error { return errors.New("disk exploded") },
	}
	if r, err := im.ImportReviewFile(ctx, w.slotID, w.dlID, false, domain.System); err != nil || r.Outcome == "placed" {
		t.Fatalf("review import %+v %v", r, err)
	}
	if sl := slot308(t, w); ptrText(sl.LastReason) == old || ptrText(sl.LastReason) != "the file could not be placed: hardlink failed (link refused), copy failed (disk exploded)" {
		t.Fatalf("the older reason stayed: %q", ptrText(sl.LastReason))
	}
	im.Ops = Ops{
		Link: func(_, _ string) error { return &os.LinkError{Op: "link", Err: fs.ErrPermission} },
		Copy: func(src, _ string) error { return &fs.PathError{Op: "open", Path: src, Err: fs.ErrPermission} },
	}
	r, err := im.ImportReviewFile(ctx, w.slotID, w.dlID, false, domain.System)
	if err != nil || r.Outcome != "failed" || r.Error != noReadWhy {
		t.Fatalf("review import %+v %v", r, err)
	}
	sl := slot308(t, w)
	if sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != noReadWhy {
		t.Fatalf("slot %s reason %q, want %q", sl.State, ptrText(sl.LastReason), noReadWhy)
	}
	if got := slotSince(t, w); got != since {
		t.Fatalf("the card's time moved from %s to %s", since, got)
	}
}

// A movie that no longer waits in Review keeps its reason when Import this file fails.
func TestReviewImportLeavesAMovieThatMovedOnAlone(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	if err := execSQL(w.s, "UPDATE slots SET state = 'searching', last_reason = 'searching' WHERE id = ?", w.slotID); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s, Ops: Ops{
		Link: func(_, _ string) error { return &os.LinkError{Op: "link", Err: fs.ErrPermission} },
		Copy: func(src, _ string) error { return &fs.PathError{Op: "open", Path: src, Err: fs.ErrPermission} },
	}}
	if r, err := im.ImportReviewFile(ctx, w.slotID, w.dlID, false, domain.System); err != nil || r.Error != noReadWhy {
		t.Fatalf("review import %+v %v", r, err)
	}
	if sl := slot308(t, w); ptrText(sl.LastReason) != "searching" {
		t.Fatalf("reason %q", ptrText(sl.LastReason))
	}
}

func slotSince(t *testing.T, w world) string {
	t.Helper()
	var at string
	if err := w.s.Read().QueryRow("SELECT updated_at FROM slots WHERE id = ?", w.slotID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}
