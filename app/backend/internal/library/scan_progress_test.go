package library

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A scan of a big Library folder tells how far it is, folder by
// folder, out of the folders this scan reads (the limit, not all of them),
// and ends at all of them, so the page keeps waiting and shows it moving.
func TestPlanAdoptReportsProgress(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for i := range 13 {
		writeFile(t, filepath.Join(lf.Path, fmt.Sprintf("Movie %02d (2001)", i), "m.mkv"), "m")
	}
	writeFile(t, filepath.Join(lf.Path, ".hidden", "m.mkv"), "m")
	ns := mediafiles.NamingDefaults()
	ns.AdoptScanLimit = 10
	if err := mediafiles.PutNamingSettings(context.Background(), s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	var got [][2]int
	ctx := WithScanProgress(context.Background(), func(done, total int) { got = append(got, [2]int{done, total}) })
	if _, err := PlanAdoptFrom(ctx, s.Read(), adoptTMDB, lf.ID, ""); err != nil {
		t.Fatal(err)
	}
	if len(got) != 11 || got[0] != [2]int{0, 10} || got[len(got)-1] != [2]int{10, 10} {
		t.Fatalf("first scan progress %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i][0] < got[i-1][0] {
			t.Fatalf("progress went back %v", got)
		}
	}
	got = nil
	if _, err := PlanAdoptFrom(ctx, s.Read(), adoptTMDB, lf.ID, "Movie 09 (2001)"); err != nil {
		t.Fatal(err)
	}
	if got[0] != [2]int{0, 3} || got[len(got)-1] != [2]int{3, 3} {
		t.Fatalf("the rest progress %v", got)
	}
	// Without a hook the scan runs as before.
	if p, err := PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, ""); err != nil || len(p.Folders) != 10 {
		t.Fatalf("plain scan %d %v", len(p.Folders), err)
	}
}
