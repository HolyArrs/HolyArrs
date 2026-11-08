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

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
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
		name, year := FolderName(e.Name())
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
