package library

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

func runSQL(t *testing.T, s *database.Store, q string, args ...any) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// One file of a movie goes to the holding folder; the movie, its other
// files and its folder stay, and the Slot it filled wants a copy again.
func TestRemoveOneFileOfAMovie(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	sl := slotsOf(t, s, id)[0]
	runSQL(t, s, "UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", fs[0].ID, sl.ID)

	before := s.WriteCount()
	preview, err := svc.RemoveFileOf(ctx, id, fs[0].ID, RemoveChoice{DeleteFiles: true}, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if s.WriteCount() != before || !onDisk(lf, fs[0].RelativePath) {
		t.Fatal("the dry run changed something")
	}
	if preview.Scope != "file" || len(preview.Held) != 1 || preview.Held[0].FileID != fs[0].ID {
		t.Fatalf("preview %+v", preview)
	}

	plan, err := svc.RemoveFileOf(ctx, id, fs[0].ID, RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	h := plan.Held[0]
	if !plan.Applied || h.To != preview.Held[0].To || onDisk(lf, h.Path) || !onDisk(lf, h.To) || !strings.HasPrefix(h.To, importer.HoldingDir+"/") {
		t.Fatalf("plan %+v", plan)
	}
	if f := fileRow(t, s, fs[0].ID); f.State != "gone" || f.RelativePath != h.To {
		t.Fatalf("file row %s %s", f.State, f.RelativePath)
	}
	for _, f := range fs[1:] {
		if !onDisk(lf, f.RelativePath) || fileRow(t, s, f.ID).State != "present" {
			t.Fatalf("%s was touched", f.RelativePath)
		}
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.State == "removed" {
		t.Fatal("the movie was deleted")
	}
	got := slotsOf(t, s, id)[0]
	if got.State != mediafiles.SlotWanted || got.FileID != nil {
		t.Fatalf("slot %s %v", got.State, got.FileID)
	}
	var kind string
	_ = s.ReadEvents().QueryRowContext(ctx, "SELECT kind FROM events WHERE kind = ? AND title_id = ?", domain.EventMovieFileRemoved, id).Scan(&kind)
	if kind != domain.EventMovieFileRemoved {
		t.Fatal("no event")
	}
	// It is gone now: deleting it again finds nothing.
	if _, err := svc.RemoveFileOf(ctx, id, fs[0].ID, RemoveChoice{DeleteFiles: true}, true, domain.System); !errors.Is(err, ErrNoSuchFile) {
		t.Fatalf("again: %v", err)
	}
}

// A file HolyArrs only found, one in a look-only folder and a protected one
// are never deleted; the refusal names why and nothing changes.
func TestRemoveOneFileRefusals(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()

	_, err := svc.RemoveFileOf(ctx, id, fs[2].ID, RemoveChoice{DeleteFiles: true}, true, domain.System)
	var kept ErrFileKept
	if !errors.As(err, &kept) || !strings.Contains(kept.Why, "found") || !onDisk(lf, fs[2].RelativePath) {
		t.Fatalf("found file: %v", err)
	}

	runSQL(t, s, "UPDATE files SET protected = 1 WHERE id = ?", fs[1].ID)
	if _, err := svc.RemoveFileOf(ctx, id, fs[1].ID, RemoveChoice{DeleteFiles: true}, true, domain.System); !errors.Is(err, ErrProtectedFile) || !onDisk(lf, fs[1].RelativePath) {
		t.Fatalf("protected: %v", err)
	}

	runSQL(t, s, "UPDATE library_folders SET look_only = 1 WHERE id = ?", lf.ID)
	_, err = svc.RemoveFileOf(ctx, id, fs[0].ID, RemoveChoice{DeleteFiles: true}, true, domain.System)
	if !errors.As(err, &kept) || !strings.Contains(kept.Why, "look-only") || !onDisk(lf, fs[0].RelativePath) {
		t.Fatalf("look-only: %v", err)
	}

	if _, err := svc.RemoveFileOf(ctx, id, 99999, RemoveChoice{DeleteFiles: true}, false, domain.System); !errors.Is(err, ErrNoSuchFile) {
		t.Fatalf("not this movie's: %v", err)
	}
}

func TestRemoveScopeFileIsOnePart(t *testing.T) {
	one := int64(1)
	if (RemoveScope{FileID: &one}).Whole() {
		t.Fatal("a file scope is not the whole title")
	}
	if err := (RemoveScope{FileID: &one, TrackIDs: []int64{2}}).Check(); !errors.Is(err, ErrMixedScope) {
		t.Fatalf("mixed: %v", err)
	}
}
