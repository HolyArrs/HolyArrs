package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestMovePlanRefusesLinksOutsideLibraryFolders(t *testing.T) {
	for _, end := range []string{"source", "destination"} {
		t.Run(end, func(t *testing.T) {
			_, s, from, id := renameWorld(t)
			to := addFolder(t, s)
			rel := "Dune - Part Two (2024)/movie.mkv"
			putFile(t, s, from, id, rel, "Dune.Part.Two.2024.1080p.WEB-DL", "imported")
			outside := t.TempDir()
			target := outside
			link := filepath.Join(to.Path, filepath.Dir(rel))
			if end == "source" {
				link = filepath.Join(from.Path, filepath.Dir(rel))
				if err := os.Rename(link, filepath.Join(outside, "original")); err != nil {
					t.Fatal(err)
				}
				target = filepath.Join(outside, "original")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			plans, err := PlanMove(context.Background(), s.Read(), s.Read(), id, to.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(plans) != 1 || plans[0].Blocked == "" {
				t.Fatalf("a %s link outside its Library folder was allowed: %+v", end, plans)
			}
		})
	}
}

func TestRenameRecoveryRefusesUnrelatedDestinationForMissingSource(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	rel := subDir + "x.mkv"
	f := putFile(t, s, lf, id, rel, "Dune.Part.Two.2024.1080p.WEB-DL", "imported")
	from := filepath.Join(lf.Path, filepath.FromSlash(rel))
	toRel := subDir + "y.mkv"
	to := filepath.Join(lf.Path, filepath.FromSlash(toRel))
	proof, err := fileProof(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO rename_steps (created_at, file_id, from_path, to_path, to_rel, state, error)
			VALUES ('2026-10-15T00:00:00Z', ?, ?, ?, ?, 'pending', ?)`, f.ID, from, to, toRel, proof)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(from); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, []byte("unrelated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverRenames(ctx, domain.System); err == nil {
		t.Fatal("unrelated destination was accepted as the recorded move")
	}
	var gotPath, state string
	if err := s.Read().QueryRow("SELECT relative_path FROM files WHERE id = ?", f.ID).Scan(&gotPath); err != nil {
		t.Fatal(err)
	}
	if err := s.Read().QueryRow("SELECT state FROM rename_steps WHERE file_id = ?", f.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if gotPath != rel || state != "failed" {
		t.Fatalf("file path %q, step %q; want original path and failed", gotPath, state)
	}
}

func TestMoveDoesNotChangeOwnershipWhenSourceIsMissing(t *testing.T) {
	_, s, from, id := renameWorld(t)
	to := addFolder(t, s)
	rel := "Dune - Part Two (2024)/movie.mkv"
	f := putFile(t, s, from, id, rel, "Dune.Part.Two.2024.1080p.WEB-DL", "imported")
	plans, err := PlanMove(context.Background(), s.Read(), s.Read(), id, to.ID)
	if err != nil || len(plans) != 1 || len(plans[0].Steps) != 1 {
		t.Fatalf("plan %+v: %v", plans, err)
	}
	step := plans[0].Steps[0]
	if err := os.Remove(step.From); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(step.To), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(step.To, []byte("unrelated"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, _, _ := runQueue(t, s, nil)
	if _, err := svc.Move(context.Background(), id, to.ID, true, domain.System); err != nil {
		t.Fatal(err)
	}
	jobs := waitJobs(t, s, 1)
	if len(jobs) != 1 || jobs[0].State != "failed" {
		t.Fatalf("move job %+v, want failed", jobs)
	}
	var folderID int64
	if err := s.Read().QueryRow("SELECT library_folder_id FROM versions WHERE id = ?", f.VersionID).Scan(&folderID); err != nil {
		t.Fatal(err)
	}
	if folderID != from.ID {
		t.Fatalf("version changed to Library folder %d after no file moved; want %d", folderID, from.ID)
	}
}
