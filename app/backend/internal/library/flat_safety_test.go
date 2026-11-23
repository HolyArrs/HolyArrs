package library

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Safety in a flat folder: deleting or renaming one movie never takes,
// renames or replaces a file of another movie sharing the folder.

var flatTwins = &metadata.Static{Movies: []metadata.Movie{
	{Name: "Heat", Year: 1995, IDs: map[string]string{"tmdb": "949"}, DigitalDate: d("1996-01-01")},
	{Name: "Heat", Year: 1986, IDs: map[string]string{"tmdb": "1001"}, DigitalDate: d("1987-01-01")},
	{Name: "Alien", Year: 1979, IDs: map[string]string{"tmdb": "348"}, DigitalDate: d("1980-01-01")},
}}

func twinsFolder(t *testing.T) (*Service, string, map[string]int64) {
	t.Helper()
	s := open(t)
	lf := addFolder(t, s)
	on := true
	var err error
	if lf, err = UpdateFolder(context.Background(), s, lf.ID, FolderChange{Flat: &on}, domain.System); err != nil {
		t.Fatal(err)
	}
	sub := "1\n00:00:01,000 --> 00:00:02,000\nx\n"
	writeFile(t, filepath.Join(lf.Path, "Heat.1995.1080p.mkv"), strings.Repeat("h", 50))
	writeFile(t, filepath.Join(lf.Path, "Heat.1995.1080p.en.srt"), sub)
	writeFile(t, filepath.Join(lf.Path, "Heat (1986).mkv"), strings.Repeat("o", 40))
	writeFile(t, filepath.Join(lf.Path, "Heat (1986).en.srt"), sub)
	writeFile(t, filepath.Join(lf.Path, "Heat.1995.1080p.Extended.en.srt"), sub) // not a subtitle of Heat.1995.1080p
	writeFile(t, filepath.Join(lf.Path, "Alien (1979).mkv"), strings.Repeat("a", 30))
	// Nested movie folder inside the flat folder.
	writeFile(t, filepath.Join(lf.Path, "Alien (1979)", "keep.txt"), "k")
	svc := &Service{Store: s, Metadata: flatTwins, Now: func() time.Time { return now }}
	res, err := svc.Adopt(context.Background(), lf.ID, []AdoptChoice{
		{Folder: "Heat.1995.1080p.mkv", TMDB: "949"}, {Folder: "Heat (1986).mkv", TMDB: "1001"}, {Folder: "Alien (1979).mkv", TMDB: "348"}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for i, k := range []string{"heat95", "heat86", "alien"} {
		if res[i].Error != "" || res[i].TitleID == 0 {
			t.Fatalf("adopt %s: %+v", k, res[i])
		}
		ids[k] = res[i].TitleID
	}
	return svc, lf.Path, ids
}

// Same title, other year: deleting Heat (1995) leaves Heat (1986), its
// subtitle, and a look-alike subtitle that is not Heat (1995)'s.
func TestFlatDeleteSameTitleOtherYear(t *testing.T) {
	svc, lib, ids := twinsFolder(t)
	before := snapshot(t, lib)
	plan, err := svc.Remove(context.Background(), ids["heat95"], RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Held) != 1 || plan.Held[0].Path != "Heat.1995.1080p.mkv" {
		t.Fatalf("held %+v", plan.Held)
	}
	after := snapshot(t, lib)
	for _, keep := range []string{"Heat (1986).mkv", "Heat (1986).en.srt", "Heat.1995.1080p.Extended.en.srt", "Alien (1979).mkv", "Alien (1979)/keep.txt"} {
		if before[keep] == "" || after[keep] != before[keep] {
			t.Fatalf("%s changed or went", keep)
		}
	}
	for _, gone := range []string{"Heat.1995.1080p.mkv", "Heat.1995.1080p.en.srt"} {
		if _, ok := after[gone]; ok {
			t.Fatalf("%s still there", gone)
		}
	}
	// Deleting Alien keeps the folder named like it and what is in it.
	if _, err := svc.Remove(context.Background(), ids["alien"], RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lib, "Alien (1979)", "keep.txt")); err != nil {
		t.Fatal("a folder of the flat Library folder was touched")
	}
}

// A rename never replaces another file, also one whose name differs only by case.
func TestFlatRenameNeverReplaces(t *testing.T) {
	svc, lib, ids := twinsFolder(t)
	ctx := context.Background()
	plan, err := PlanRename(ctx, svc.Store.Read(), []int64{ids["heat95"]})
	if err != nil {
		t.Fatal(err)
	}
	var to string
	for _, st := range plan {
		if st.Skip == "" {
			to = st.To
		}
	}
	if to == "" {
		t.Skipf("nothing to rename: %+v", plan)
	}
	occupant := to
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		occupant = strings.ToUpper(to) // the same entry on a case-blind disk
	}
	writeFile(t, filepath.Join(lib, occupant), "someone else")
	before := snapshot(t, lib)
	plan, err = PlanRename(ctx, svc.Store.Read(), []int64{ids["heat95"], ids["heat86"], ids["alien"]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil {
		t.Logf("apply: %v", err)
	}
	after := snapshot(t, lib)
	if before[occupant] == "" || after[occupant] != before[occupant] {
		t.Fatalf("%s was replaced: %q", occupant, after[occupant])
	}
	if after["Heat.1995.1080p.mkv"] != before["Heat.1995.1080p.mkv"] {
		t.Fatal("the movie's file was lost")
	}
	if len(after) != len(before) {
		t.Fatalf("files %d, were %d", len(after), len(before))
	}
}

// A link in a flat folder: it is either not adopted or, when deleted,
// only the link moves; the file it points at stays.
func TestFlatDeleteLinkKeepsTarget(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	on := true
	lf, _ = UpdateFolder(context.Background(), s, lf.ID, FolderChange{Flat: &on}, domain.System)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "Alien (1979).mkv"), strings.Repeat("a", 30))
	if err := os.Symlink(filepath.Join(outside, "Alien (1979).mkv"), filepath.Join(lf.Path, "Alien (1979).mkv")); err != nil {
		t.Skipf("no links here: %v", err)
	}
	svc := &Service{Store: s, Metadata: flatTwins, Now: func() time.Time { return now }}
	res, err := svc.Adopt(context.Background(), lf.ID, []AdoptChoice{{Folder: "Alien (1979).mkv", TMDB: "348"}}, domain.System)
	if err != nil || len(res) != 1 {
		t.Fatal(err)
	}
	if res[0].TitleID == 0 {
		// A link is not adopted as a flat movie: nothing to delete, and
		// the file it points at is never touched.
		if fi, err := os.Stat(filepath.Join(outside, "Alien (1979).mkv")); err != nil || fi.Size() != 30 {
			t.Fatal("the file the link points at was touched")
		}
		return
	}
	if _, err := svc.Remove(context.Background(), res[0].TitleID, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(outside, "Alien (1979).mkv")); err != nil || fi.Size() != 30 {
		t.Fatal("the file the link points at was touched")
	}
}
