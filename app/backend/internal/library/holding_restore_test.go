package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A deleted movie's held files are listed with where they came from, and a
// restore puts them back and brings the movie back to the Library.
func TestRestoreHeldBringsTheMovieBack(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	if _, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	held, err := svc.HeldFiles(ctx)
	if err != nil || len(held) != 2 {
		t.Fatalf("held %+v %v", held, err)
	}
	h := held[0]
	if !h.TitleDeleted || h.TitleID != id || h.ExpiresAt == nil || h.ExpiresAt.Format("2006-01-02") != "2026-10-21" {
		t.Fatalf("held %+v", h)
	}
	// A taken place is never overwritten.
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(fs[1].RelativePath)), "someone")
	res, back, err := svc.RestoreHeld(ctx, []int64{fs[0].ID, fs[1].ID, 999}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 || res[0].Result != "restored" || !strings.Contains(res[1].Result, "already") || res[2].Result != "not in a holding folder" {
		t.Fatalf("results %+v", res)
	}
	if len(back) != 1 || back[0] != id {
		t.Fatalf("back %v", back)
	}
	if !onDisk(lf, fs[0].RelativePath) {
		t.Fatal("not back on disk")
	}
	if f := fileRow(t, s, fs[0].ID); f.State != "present" || f.RelativePath != fs[0].RelativePath {
		t.Fatalf("row %+v", f)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.State != "active" {
		t.Fatalf("title %s", ti.State)
	}
	if sl := slotsOf(t, s, id); sl[0].State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s", sl[0].State)
	}
	if b, _ := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(fs[1].RelativePath))); string(b) != "someone" {
		t.Fatal("overwrote a file")
	}
	if held, _ := svc.HeldFiles(ctx); len(held) != 1 || held[0].ID != fs[1].ID {
		t.Fatalf("still held %+v", held)
	}
}
