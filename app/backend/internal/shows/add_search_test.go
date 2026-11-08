package shows

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// TestAddShowSearchOnAdd: a show added with search_on_add gets, after its
// folder scan, an automatic search of each season looking for a copy: no
// by_hand, so the release gate still holds back what is not out yet.
func TestAddShowSearchOnAdd(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, jobID, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{SearchOnAdd: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var scan domain.Job
	if err := database.Get(ctx, s.Read(), "jobs", jobID, &scan); err != nil {
		t.Fatal(err)
	}
	if scan.Kind != library.KindScanTitle {
		t.Fatalf("first job %s", scan.Kind)
	}
	if n := countJobs(t, s, "search_season"); n != 0 {
		t.Fatalf("search before the scan: %d", n)
	}
	if err := svc.RunNextStep(ctx, &jobs.Run{Job: scan}, id); err != nil {
		t.Fatal(err)
	}
	ss, err := database.Select[domain.Job](ctx, s.Read(), "jobs", "kind = 'search_season' ORDER BY id")
	if err != nil || len(ss) == 0 {
		t.Fatalf("season searches %v %v", ss, err)
	}
	for _, j := range ss {
		if _, hand := j.Params["by_hand"]; hand || j.Params["kept_only"] != nil || j.Params["version_id"] == nil || j.Subject == nil || j.Subject.Type != "part" {
			t.Fatalf("season search %+v", j)
		}
	}
	// Without search_on_add the scan is the last step.
	svc2, s2, _, _ := addShowService(t)
	addShowFolder(t, s2)
	id2, jobID2, err := svc2.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var scan2 domain.Job
	_ = database.Get(ctx, s2.Read(), "jobs", jobID2, &scan2)
	if err := svc2.RunNextStep(ctx, &jobs.Run{Job: scan2}, id2); err != nil {
		t.Fatal(err)
	}
	if n := countJobs(t, s2, "search_season"); n != 0 {
		t.Fatalf("searched without search_on_add: %d", n)
	}
}
