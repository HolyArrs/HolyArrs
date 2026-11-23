package shows

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestShowFolderOK(t *testing.T) {
	for f, want := range map[string]bool{"The Office": true, "O/The Office": true, "A/B/C": false, "../x": false, "O/..": false,
		".hidden": false, `O\x`: false, "": false, "O/ x": false, "/abs": false} {
		if got := showFolderOK(f); got != want {
			t.Errorf("showFolderOK(%q) = %v, want %v", f, got, want)
		}
	}
}

// A show in a letter folder is scanned and adopted where it is.
func TestAdoptShowInLetterFolder(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "L", "Lab Show (2025)", "Season 1", "Lab.Show.S01E01.1080p.WEB-DL.H264-GRP.mkv"), strings.Repeat("a", 300))
	before := snapshot(t, lf.Path)
	p, err := PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	c := byFolder(p)["L/Lab Show (2025)"]
	if p.Total != 1 || c.Status != "matched" || c.Match == nil || c.Match.ID != "tmdb:100" || c.Episodes != 1 {
		t.Fatalf("plan %+v / %+v", p, c)
	}
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "L/Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || len(res) != 1 || res[0].Error != "" || res[0].Episodes != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", res[0].TitleID)
	if len(vs) != 1 || vs[0].FolderName != "L/Lab Show (2025)" {
		t.Fatalf("versions %+v", vs)
	}
	if !reflect.DeepEqual(snapshot(t, lf.Path), before) {
		t.Fatal("adopting moved a file")
	}
	if p, _ = PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, lf.ID, ""); byFolder(p)["L/Lab Show (2025)"].Status != "in_library" {
		t.Fatalf("second scan %+v", p.Folders)
	}
}
