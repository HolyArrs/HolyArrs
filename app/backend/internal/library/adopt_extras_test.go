package library

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestMovieExtra(t *testing.T) {
	for rel, want := range map[string]bool{
		"Trailers/trailer.mkv":             true,
		"Behind.The.Scenes/making.mkv":     true,
		"Featurettes/cast.mkv":             true,
		"extras/deep/inside/a.mkv":         true,
		"Heat (1995)-trailer.mkv":          true,
		"heat-behindthescenes.mp4":         true,
		"trailer.mp4":                      true,
		"Heat.1995.1080p.BluRay.mkv":       false,
		"BDMV/STREAM/00001.m2ts":           false,
		"Disc 1/heat.mkv":                  false,
		"Feature/heat.mkv":                 false,
		"Trailer.Park.Boys.2006.1080p.mkv": false,
		"The.Deleted.2020.1080p.mkv":       false,
	} {
		if got := MovieExtra(rel); got != want {
			t.Errorf("MovieExtra(%q) = %v, want %v", rel, got, want)
		}
	}
}

// A movie folder that holds only a trailer and a featurette has no movie
// file: adopting it must not take an extra as the movie, which a later
// upgrade or delete would then treat as the movie's own copy. In a folder
// with the movie, a bigger extra never wins over it.
func TestAdoptNeverTakesAnExtraAsTheMovie(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Empty Movie (2001)", "Trailers", "trailer.mkv"), strings.Repeat("t", 500))
	writeFile(t, filepath.Join(lf.Path, "Empty Movie (2001)", "Empty Movie (2001)-featurette.mkv"), strings.Repeat("f", 400))
	writeFile(t, filepath.Join(lf.Path, "Heat (1995) {tmdb-949}", "heat.avi"), strings.Repeat("h", 100))
	writeFile(t, filepath.Join(lf.Path, "Heat (1995) {tmdb-949}", "Featurettes", "making-of.mkv"), strings.Repeat("m", 1000))
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	ctx := context.Background()

	plan, err := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := byFolder(plan)
	if c := got["Empty Movie (2001)"]; c.Main != "" || c.Status != "no_video" {
		t.Fatalf("an extras-only folder was offered with %q as the movie (%s)", c.Main, c.Status)
	}
	if m := got["Heat (1995) {tmdb-949}"].Main; m != "Heat (1995) {tmdb-949}/heat.avi" {
		t.Fatalf("main file %q", m)
	}

	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Empty Movie (2001)", TMDB: "5001"}, {Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].TitleID == 0 || res[1].TitleID == 0 {
		t.Fatalf("results %+v", res)
	}
	files, _ := database.Select[mediafiles.File](ctx, s.Read(), "files", "origin = 'adopted' ORDER BY id")
	if len(files) != 1 || files[0].RelativePath != "Heat (1995) {tmdb-949}/heat.avi" {
		t.Fatalf("adopted files %+v", files)
	}
	if sl := slotsOf(t, s, res[0].TitleID)[0]; sl.FileID != nil || sl.State != mediafiles.SlotWanted {
		t.Fatalf("the extras-only movie got a file: %+v", sl)
	}

	// A rescan finds the extras, but never links one as the movie's copy.
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := slotsOf(t, s, res[0].TitleID)[0]; sl.FileID != nil {
		t.Fatalf("a rescan linked an extra as the movie: %+v", sl)
	}
}

// Fifty extras read before the movie do not hide it: extras do not count
// toward the read limit when the movie file is picked.
func TestManyExtrasDoNotHideTheMovie(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for i := 0; i < maxScanVideos+5; i++ {
		writeFile(t, filepath.Join(lf.Path, "Heat (1995) {tmdb-949}", "Extras", fmt.Sprintf("clip%02d.mkv", i)), "x")
	}
	writeFile(t, filepath.Join(lf.Path, "Heat (1995) {tmdb-949}", "heat.mkv"), strings.Repeat("h", 10))
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	ctx := context.Background()
	plan, err := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m := byFolder(plan)["Heat (1995) {tmdb-949}"].Main; m != "Heat (1995) {tmdb-949}/heat.mkv" {
		t.Fatalf("main file %q", m)
	}
	if _, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}, domain.System); err != nil {
		t.Fatal(err)
	}
	files, _ := database.Select[mediafiles.File](ctx, s.Read(), "files", "origin = 'adopted'")
	if len(files) != 1 || files[0].RelativePath != "Heat (1995) {tmdb-949}/heat.mkv" {
		t.Fatalf("adopted %+v", files)
	}
}
