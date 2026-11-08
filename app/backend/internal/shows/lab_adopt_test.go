package shows

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestLabShowsAdoptedWithNoFileMoved adopts a copy of the lab show folder
// (5.2.2 done-when): every lab show is matched, every episode file is mapped
// to its episode, and no file moves. The lab folder itself is only read,
// before and after. Run with
//
//	HOLYARRS_LAB_TV=<holyarrs>/research/lab/media/tv go test -run LabShows ./internal/library/
func TestLabShowsAdoptedWithNoFileMoved(t *testing.T) {
	src := os.Getenv("HOLYARRS_LAB_TV")
	if src == "" {
		t.Skip("HOLYARRS_LAB_TV not set")
	}
	labBefore := snapshot(t, src)
	svc, s, tmdb, _ := addShowService(t)
	lf := addShowFolder(t, s)
	copyTree(t, src, lf.Path)
	before := snapshot(t, lf.Path)

	// A fake details source that knows each lab show by its folder's name
	// and year, with two seasons of 60 episodes.
	ents, _ := os.ReadDir(lf.Path)
	files := 0
	for i, e := range ents {
		name, year := library.FolderName(e.Name())
		id := fmt.Sprint(700000 + i)
		sh := metadata.Show{Source: "tmdb", Name: name, Year: year, Status: "ended", IDs: map[string]string{"tmdb": id}}
		for se := 1; se <= 2; se++ {
			for ep := 1; ep <= 60; ep++ {
				sh.Episodes = append(sh.Episodes, metadata.Episode{Season: se, Number: ep, AirDate: tm("2010-01-01T00:00:00Z"), AirDay: "2010-01-01", ID: fmt.Sprintf("%s-%d-%d", id, se, ep)})
			}
		}
		tmdb.set(sh, "tmdb:"+id)
		_ = filepath.WalkDir(filepath.Join(lf.Path, e.Name()), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".mkv") {
				files++
			}
			return nil
		})
	}
	ctx := context.Background()
	plan, err := PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	var cs []library.AdoptChoice
	mapped := 0
	for _, c := range plan.Folders {
		if c.Status != "matched" || c.Unknown != 0 {
			t.Errorf("%s: %s (%s), %d files with no episode", c.Folder, c.Status, c.Why, c.Unknown)
			continue
		}
		mapped += c.Episodes
		cs = append(cs, library.AdoptChoice{Folder: c.Folder, ID: c.Match.ID})
	}
	res, err := svc.Adopt(ctx, lf.ID, cs, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	adopted := 0
	for _, r := range res {
		if r.TitleID == 0 || r.Error != "" || len(r.Unknown) != 0 {
			t.Errorf("%s: %+v", r.Folder, r)
		}
		adopted += r.Episodes
	}
	if mapped != files || adopted != files {
		t.Errorf("%d video files, %d mapped, %d adopted", files, mapped, adopted)
	}
	if fmt.Sprint(snapshot(t, lf.Path)) != fmt.Sprint(before) {
		t.Fatal("adopting moved or changed a file")
	}
	if fmt.Sprint(snapshot(t, src)) != fmt.Sprint(labBefore) {
		t.Fatal("the lab folder changed")
	}
	t.Logf("adopted %d lab shows with %d episodes, %d files untouched", len(res), adopted, len(before))
}
