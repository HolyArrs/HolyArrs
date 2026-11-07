package library

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestLabLibraryAdoptedWithNoFileMoved adopts a copy of the lab movie
// folder (5.1.2 done-when). The lab folder itself is only read, before and
// after. Run with
//
//	HOLYARRS_LAB_MEDIA=<holyarrs>/research/lab/media/movies go test -run LabLibrary ./internal/library/
func TestLabLibraryAdoptedWithNoFileMoved(t *testing.T) {
	src := os.Getenv("HOLYARRS_LAB_MEDIA")
	if src == "" {
		t.Skip("HOLYARRS_LAB_MEDIA not set")
	}
	labBefore := snapshot(t, src)
	s := open(t)
	lf := addFolder(t, s)
	copyTree(t, src, lf.Path)
	before := snapshot(t, lf.Path)

	// A fake TMDB that knows each lab movie by its folder's name and year.
	md := &metadata.Static{}
	ents, _ := os.ReadDir(lf.Path)
	for i, e := range ents {
		name, year := folderName(e.Name())
		md.Movies = append(md.Movies, metadata.Movie{Name: name, Year: year, IDs: map[string]string{"tmdb": fmt.Sprint(800000 + i)},
			DigitalDate: d(fmt.Sprintf("%04d-06-01", max(year, 1900)))})
	}
	svc := &Service{Store: s, Metadata: md, Now: func() time.Time { return now }}
	ctx := context.Background()
	plan, err := PlanAdopt(ctx, s.Read(), md, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	var cs []AdoptChoice
	for _, c := range plan.Folders {
		if c.Status != "matched" {
			t.Errorf("%s: %s (%s)", c.Folder, c.Status, c.Why)
			continue
		}
		cs = append(cs, AdoptChoice{Folder: c.Folder, TMDB: c.Match.TMDB})
	}
	res, err := svc.Adopt(ctx, lf.ID, cs, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.TitleID == 0 || r.File == "" {
			t.Errorf("%s: %+v", r.Folder, r)
		}
	}
	if fmt.Sprint(snapshot(t, lf.Path)) != fmt.Sprint(before) {
		t.Fatal("adopting moved or changed a file")
	}
	if fmt.Sprint(snapshot(t, src)) != fmt.Sprint(labBefore) {
		t.Fatal("the lab folder changed")
	}
	t.Logf("adopted %d lab movies, %d files untouched", len(res), len(before))
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		if e.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		if strings.HasPrefix(e.Name(), ".") {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(to)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
}

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
		name, year := folderName(e.Name())
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
	var cs []AdoptChoice
	mapped := 0
	for _, c := range plan.Folders {
		if c.Status != "matched" || c.Unknown != 0 {
			t.Errorf("%s: %s (%s), %d files with no episode", c.Folder, c.Status, c.Why, c.Unknown)
			continue
		}
		mapped += c.Episodes
		cs = append(cs, AdoptChoice{Folder: c.Folder, ID: c.Match.ID})
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
