package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

func protectFile(t *testing.T, s *database.Store, id int64) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET protected=1 WHERE id=?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedMovieRemovalNeedsExplicitDeleteAcknowledgement(t *testing.T) {
	svc, s, lf, titleID, files := deleteWorld(t)
	ctx := context.Background()
	protectFile(t, s, files[0].ID)

	preview, err := svc.Remove(ctx, titleID, RemoveChoice{DeleteFiles: true}, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Skipped || len(preview.Protected) != 1 || len(preview.Held) != 0 {
		t.Fatalf("preview %+v", preview)
	}
	if _, err := svc.Remove(ctx, titleID, RemoveChoice{DeleteFiles: true}, true, domain.System); !errors.Is(err, ErrProtectedCopy) {
		t.Fatalf("remove error %v", err)
	}
	var title mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", titleID, &title); err != nil {
		t.Fatal(err)
	}
	if title.State == "removed" || !onDisk(lf, files[0].RelativePath) || !fileRow(t, s, files[0].ID).Protected {
		t.Fatalf("protected movie changed: title=%s file=%+v", title.State, fileRow(t, s, files[0].ID))
	}

	plan, err := svc.Remove(ctx, titleID, RemoveChoice{DeleteFiles: true, AcknowledgeProtection: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Skipped || len(plan.Held) != 2 || onDisk(lf, files[0].RelativePath) {
		t.Fatalf("acknowledged plan %+v", plan)
	}
	if f := fileRow(t, s, files[0].ID); f.State != "gone" || f.RelativePath == files[0].RelativePath || f.Protected {
		t.Fatalf("held protected file %+v", f)
	}
}

func TestHoldingCleanupKeepsDayContainingProtectedCopy(t *testing.T) {
	svc, s, lf, _, files := deleteWorld(t)
	ctx := context.Background()
	rel := importer.HoldingDir + "/2026-10-01/Dune - Part Two (2024)/protected.mkv"
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state='replaced', relative_path=?, protected=1 WHERE id=?", rel, files[0].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "protected bytes")
	writeFile(t, filepath.Join(lf.Path, importer.HoldingDir, "2026-10-01", "other.txt"), "other")

	n, err := svc.CleanHolding(ctx, domain.System)
	if err != nil || n != 0 {
		t.Fatalf("cleaned %d: %v", n, err)
	}
	b, err := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(rel)))
	if err != nil || string(b) != "protected bytes" {
		t.Fatalf("protected holding copy changed: %q %v", b, err)
	}
	if f := fileRow(t, s, files[0].ID); f.State != "replaced" || !f.Protected {
		t.Fatalf("protected row %+v", f)
	}
}

func TestProtectionPersistsWhenMissingAndAcrossRestart(t *testing.T) {
	svc, s, lf, titleID := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, titleID, "Dune - Part Two (2024)/protected.mkv", "Dune.Part.Two.2024.1080p.WEB-HOLYARRS", "imported")
	yes := true
	if _, err := svc.SetFileChoice(ctx, titleID, f.ID, FileChoice{Protected: &yes}, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath))); err != nil {
		t.Fatal(err)
	}

	dir := s.Dir()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := &Service{Store: reopened, Now: func() time.Time { return now }}
	if _, err := restarted.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	missing := fileRow(t, reopened, f.ID)
	if missing.State != "missing" || !missing.Protected {
		t.Fatalf("after restart/rescan %+v", missing)
	}
}

func TestKeepAndAddVersionRequiresExplicitChoicesAndIsAtomic(t *testing.T) {
	svc, s, lf, titleID := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, titleID, "Dune - Part Two (2024)/main.mkv", "Dune.Part.Two.2024.1080p.WEB-HOLYARRS", "imported")

	if _, err := svc.KeepAndAddVersion(ctx, titleID, f.ID, VersionInput{Label: "4K"}, domain.System); !errors.Is(err, ErrCopyChoice) {
		t.Fatalf("implicit choices accepted: %v", err)
	}
	if fileRow(t, s, f.ID).Protected || len(versionsOf(t, s, titleID)) != 1 {
		t.Fatal("failed keep-and-add changed the Library")
	}

	other := addFolder(t, s)
	version, err := svc.KeepAndAddVersion(ctx, titleID, f.ID, VersionInput{Label: "4K", FolderName: "Dune Part Two 4K", PickRulesID: fourKRules(t, s), LibraryFolderID: other.ID, Edition: "Director's Cut", AudioLanguages: []string{"ja", "en"}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if !fileRow(t, s, f.ID).Protected || version.FolderName != "Dune Part Two 4K" || version.PickRulesID != fourKRules(t, s) || version.LibraryFolderID != other.ID || version.Edition != "Director's Cut" || len(version.AudioLanguages) != 2 {
		t.Fatalf("file=%+v version=%+v", fileRow(t, s, f.ID), version)
	}
	if slots := slotsOf(t, s, titleID); len(slots) != 2 || slots[1].VersionID != version.ID || slots[1].State != mediafiles.SlotWanted {
		t.Fatalf("slots %+v", slots)
	}
}
