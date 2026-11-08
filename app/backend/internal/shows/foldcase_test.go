package shows

import (
	"context"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// 5.2 review: on a case-sensitive filesystem a show folder differing only
// in case from one in the Library is its own folder, not in the Library.
func TestPlanShowAdoptCaseOnlyFolder(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	if res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System); err != nil || res[0].Error != "" {
		t.Fatalf("%+v %v", res, err)
	}
	fold := library.FoldsCase(lf.Path)
	if !fold {
		writeFile(t, filepath.Join(lf.Path, "LAB SHOW (2025)", "Season 1", "Lab.Show.S01E03.mkv"), "x")
	}
	p, err := PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	c := byFolder(p)
	if c["Lab Show (2025)"].Status != "in_library" {
		t.Fatalf("adopted folder %+v", c["Lab Show (2025)"])
	}
	if fold {
		return // the other folder cannot exist here
	}
	if c["LAB SHOW (2025)"].Why == "this folder already belongs to a show in the Library" {
		t.Fatalf("case-only folder %+v", c["LAB SHOW (2025)"])
	}
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "LAB SHOW (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || (len(res) == 1 && res[0].Error == "this folder already belongs to a show in the Library") {
		t.Fatalf("adopting the case-only folder: %+v %v", res, err)
	}
}
