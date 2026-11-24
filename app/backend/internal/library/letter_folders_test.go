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

func groupByLetter(t *testing.T, s *database.Store) {
	t.Helper()
	ns, err := mediafiles.ReadNaming(context.Background(), s.Read())
	if err != nil {
		t.Fatal(err)
	}
	ns.GroupByLetter = true
	if err := mediafiles.PutNamingSettings(context.Background(), s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	if back, _ := mediafiles.ReadNaming(context.Background(), s.Read()); !back.GroupByLetter {
		t.Fatal("not saved")
	}
}

// With Group by first letter on, a new movie gets D/Title (Year)/
// and a rename moves an existing movie folder into its letter folder.
func TestGroupByLetterAddAndRename(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	ctx := context.Background()
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, s, lf, id, "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv", "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS", "imported")
	groupByLetter(t, s)
	svc := &Service{Store: s, Now: func() time.Time { return now }}
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 || filepath.ToSlash(filepath.Dir(plan[0].To)) != "D/Dune - Part Two (2024)" {
		t.Fatalf("%+v %v", plan, err)
	}
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", id)
	if vs[0].FolderName != "D/Dune - Part Two (2024)" || exists(filepath.Join(lf.Path, "Dune - Part Two (2024)")) {
		t.Fatalf("version folder %q", vs[0].FolderName)
	}
	m := dune
	m.Name, m.Year, m.IDs = "Old School", 2003, map[string]string{"tmdb": "10595"}
	id2, err := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ = database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", id2)
	if len(vs) != 1 || filepath.Dir(filepath.FromSlash(vs[0].FolderName)) != "O" {
		t.Fatalf("new movie %+v", vs)
	}
}
