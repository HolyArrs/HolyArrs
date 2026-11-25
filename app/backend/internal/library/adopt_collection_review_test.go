package library

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestCollectionRejectsDifferentOwner(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	ctx := context.Background()
	f := "Collection/Heat (1995).mkv"
	writeFile(t, filepath.Join(lf.Path, f), "heat")
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	r, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: f, TMDB: "949"}}, domain.System)
	if err != nil || r[0].Error != "" {
		t.Fatalf("first %+v %v", r, err)
	}
	r, err = svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: f, TMDB: dune.IDs["tmdb"]}}, domain.System)
	if err != nil || r[0].Error == "" || r[0].TitleID != 0 {
		t.Fatalf("wrong movie reported as imported: %+v %v", r, err)
	}
}

func TestCollectionAtLibraryRoot(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for _, n := range []string{"Heat (1995).mkv", "Dune - Part Two (2024).mkv"} {
		writeFile(t, filepath.Join(lf.Path, n), "movie")
	}
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil || p.Counts["matched"] != 2 {
		t.Fatalf("root files %+v %v", p, err)
	}
}

func TestCollectionConvertsExclusiveParentToShared(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	ctx := context.Background()
	writeFile(t, filepath.Join(lf.Path, "Collection", "Heat (1995).mkv"), "heat")
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	r, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Collection", TMDB: "949"}}, domain.System)
	if err != nil || r[0].Error != "" {
		t.Fatalf("first %+v %v", r, err)
	}
	f := "Collection/Nested/Dune - Part Two (2024).mkv"
	writeFile(t, filepath.Join(lf.Path, f), "dune")
	r, err = svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: f, TMDB: dune.IDs["tmdb"]}}, domain.System)
	if err != nil || r[0].Error != "" {
		t.Fatalf("second %+v %v", r, err)
	}
	writeFile(t, filepath.Join(lf.Path, "Collection", "Other (2000).mkv"), "other")
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	fs, err := database.Select[mediafiles.File](ctx, s.Read(), "files", "")
	if err != nil || len(fs) != 2 {
		t.Fatalf("shared parent claimed other movies: %+v %v", fs, err)
	}
}

func TestCollectionUnassignedFileIsNotImported(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	ctx := context.Background()
	writeFile(t, filepath.Join(lf.Path, "Collection", "Heat (1995).mkv"), "heat")
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	r, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Collection", TMDB: "949"}}, domain.System)
	if err != nil || r[0].Error != "" {
		t.Fatalf("first %+v %v", r, err)
	}
	f := "Collection/Dune - Part Two (2024).mkv"
	writeFile(t, filepath.Join(lf.Path, f), "dune")
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	p, err := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if err != nil || byFolder(p)[f].Status == "in_library" {
		t.Fatalf("unassigned file reported as imported: %+v %v", p, err)
	}
	r, err = svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: f, TMDB: "949"}}, domain.System)
	if err != nil || r[0].Error == "" {
		t.Fatalf("unassigned file accepted as successful import: %+v %v", r, err)
	}
}
