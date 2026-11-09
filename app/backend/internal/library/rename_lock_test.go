package library

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestApplyRenameWaitsForFileLockBeforeJournalOrMove(t *testing.T) {
	svc, s, lf, titleID := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, titleID, "Dune - Part Two (2024)/old.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	plan, err := PlanRename(ctx, s.Read(), []int64{titleID})
	if err != nil || len(plan) != 1 {
		t.Fatalf("plan %+v: %v", plan, err)
	}

	unlock := s.LockFiles()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := svc.ApplyRename(ctx, plan, 0, domain.System)
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		unlock()
		t.Fatalf("rename passed the held file lock: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	var steps int
	if err := s.Read().QueryRow("SELECT count(*) FROM rename_steps WHERE file_id = ?", f.ID).Scan(&steps); err != nil {
		unlock()
		t.Fatal(err)
	}
	if steps != 0 || fileRow(t, s, f.ID).RelativePath != f.RelativePath {
		unlock()
		t.Fatalf("rename changed state behind the lock: steps=%d file=%+v", steps, fileRow(t, s, f.ID))
	}
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("rename did not continue after the file lock was released")
	}
	if got := fileRow(t, s, f.ID).RelativePath; got != plan[0].To {
		t.Fatalf("renamed path %q, want %q", got, plan[0].To)
	}
}

func TestRecoverRenamesWaitsForFileLock(t *testing.T) {
	svc, s, lf, titleID := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, titleID, "Dune - Part Two (2024)/old.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	plan, err := PlanRename(ctx, s.Read(), []int64{titleID})
	if err != nil || len(plan) != 1 {
		t.Fatalf("plan %+v: %v", plan, err)
	}
	renameCrash = func(stage string) error {
		if stage == "after_move" {
			return errors.New("test stop after move")
		}
		return nil
	}
	_, err = svc.ApplyRename(ctx, plan, 0, domain.System)
	renameCrash = nil
	if err == nil {
		t.Fatal("rename did not leave a recovery step")
	}
	if got := fileRow(t, s, f.ID).RelativePath; got != f.RelativePath {
		t.Fatalf("cut-off rename updated the File: %q", got)
	}

	unlock := s.LockFiles()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := svc.RecoverRenames(ctx, domain.System)
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		unlock()
		t.Fatalf("recovery passed the held file lock: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	if got := fileRow(t, s, f.ID).RelativePath; got != f.RelativePath {
		unlock()
		t.Fatalf("recovery updated the File behind the lock: %q", got)
	}
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not continue after the file lock was released")
	}
	if got := fileRow(t, s, f.ID).RelativePath; got != plan[0].To {
		t.Fatalf("recovered path %q, want %q", got, plan[0].To)
	}
}
