package library

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestPlanAdoptCollectionFiles(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	root := "Collection 1995-2024/Movies"
	want := []string{root + "/Dune - Part Two (2024).mkv", root + "/Heat (1995).mkv"}
	for _, f := range want {
		writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(f)), "movie")
	}
	writeFile(t, filepath.Join(lf.Path, root, "Heat (1995).sample.mkv"), "sample")
	writeFile(t, filepath.Join(lf.Path, "Collection 1995-2024", "Images", "cover.jpg"), "art")
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := folderNames(p); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("candidates %q, want %q", got, want)
	}
	for _, c := range p.Folders {
		if c.Status != "matched" || len(c.Videos) != 1 || c.Main != c.Folder {
			t.Fatalf("candidate %+v", c)
		}
	}
}

func TestPlanAdoptCollectionEdges(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for _, f := range []string{
		"A/B/C/D/E/Heat (1995)/movie.mkv",
		"Encodes/Heat.1995.1080p.mkv", "Encodes/Heat.1995.2160p.mkv",
		"Discs/Disc 1/Heat (1995).mkv", "Discs/Disc 2/Heat (1995).mkv",
		"Collection/Dune - Part Two (2024).mkv", "Collection/Heat (1995).mkv",
		"Collection/Unknown.mkv", "Collection/Nested/Empty Movie (2001)/movie.mkv",
		"Collection/Sample/Other (2001).mkv", "Collection/Extras/Making Of (2002).mkv",
	} {
		writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(f)), "video")
	}
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A/B/C/D/E/Heat (1995)", "Collection/Dune - Part Two (2024).mkv", "Collection/Heat (1995).mkv", "Collection/Nested/Empty Movie (2001)", "Collection/Unknown.mkv", "Discs", "Encodes"}
	if fmt.Sprint(folderNames(p)) != fmt.Sprint(want) {
		t.Fatalf("got %q, want %q", folderNames(p), want)
	}
}

func TestScanCollectionFilesAcrossBatches(t *testing.T) {
	for _, batch := range []bool{true, false} {
		t.Run(fmt.Sprintf("batch=%v", batch), func(t *testing.T) {
			s := open(t)
			lf := addFolder(t, s)
			for i := 1; i <= 3*scanWindow+2; i++ {
				writeFile(t, filepath.Join(lf.Path, "Collection", fmt.Sprintf("Film %d (2001).mkv", i)), "movie")
			}
			base, _, _ := movieServer(t, batch)
			md := &metadata.TMDB{BaseURL: base + metadata.TMDBPath, Access: func(context.Context) (string, error) { return "tok", nil }, HTTP: http.DefaultClient, Gap: 1}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p, err := PlanAdopt(ctx, s.Read(), md, lf.ID)
			if err != nil || p.Counts["matched"] != 3*scanWindow+2 {
				t.Fatalf("counts %v, error %v", p.Counts, err)
			}
		})
	}
}

func TestAdoptCollectionFilesStayIndependent(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	heat, duneFile := "Collection/Heat (1995).mkv", "Collection/Dune - Part Two (2024).mkv"
	writeFile(t, filepath.Join(lf.Path, heat), "heat")
	writeFile(t, filepath.Join(lf.Path, duneFile), "dune")
	before := snapshot(t, lf.Path)
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	ctx := context.Background()
	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: heat, TMDB: "949"}}, domain.System)
	if err != nil || len(res) != 1 || res[0].Error != "" || res[0].File != heat {
		t.Fatalf("adopt %+v %v", res, err)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	files, err := database.Select[mediafiles.File](ctx, s.Read(), "files", "")
	if err != nil || len(files) != 1 || files[0].RelativePath != heat {
		t.Fatalf("files %+v %v", files, err)
	}
	p, err := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byFolder(p)[heat].Status != "in_library" || byFolder(p)[duneFile].Status != "matched" {
		t.Fatalf("plan %+v", p)
	}
	res, err = svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: duneFile, TMDB: dune.IDs["tmdb"]}, {Folder: heat, TMDB: "949"}}, domain.System)
	if err != nil || len(res) != 2 || res[0].Error != "" || res[1].Error != "" || res[0].TitleID == res[1].TitleID {
		t.Fatalf("adopt %+v %v", res, err)
	}
	if got := snapshot(t, lf.Path); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatal("adoption changed media files")
	}
	// A collection does not become a single-movie folder when one file goes missing.
	if err := os.Remove(filepath.Join(lf.Path, duneFile)); err != nil {
		t.Fatal(err)
	}
	p, err = PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if err != nil || byFolder(p)[heat].Status != "in_library" {
		t.Fatalf("remaining file %+v %v", p, err)
	}
}
