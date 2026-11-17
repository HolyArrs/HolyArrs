package library

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// nestedTree is a Library folder sorted into letter, genre and collection
// folders, with one movie folder left at the top.
func nestedTree(t *testing.T, lib string) {
	t.Helper()
	writeFile(t, filepath.Join(lib, "A-Z", "D", "Dune - Part Two (2024)", "dune.mkv"), "dune main file")
	writeFile(t, filepath.Join(lib, "A-Z", "D", "Dune - Part Two (2024)", "Featurettes", "making-of.mkv"), "f")
	writeFile(t, filepath.Join(lib, "Action", "Heat (1995)", "heat.avi"), "h")
	writeFile(t, filepath.Join(lib, "Action", "notes.txt"), "n")
	writeFile(t, filepath.Join(lib, "Lantern Collection", "The Lantern Keepers (2027)", "lk.mkv"), "l")
	writeFile(t, filepath.Join(lib, "Lantern Collection", "The Lantern Keepers Return (2029)", "lkr.mkv"), "r")
	writeFile(t, filepath.Join(lib, "Lantern Collection", ".hidden", "x.mkv"), "x")
	writeFile(t, filepath.Join(lib, "Empty Movie (2001)", "readme.txt"), "e")
	writeFile(t, filepath.Join(lib, "Lone Disc", "CD1", "part1.avi"), "1")
}

func folderNames(p AdoptPlan) []string {
	out := []string{}
	for _, c := range p.Folders {
		out = append(out, c.Folder)
	}
	sort.Strings(out)
	return out
}

// Movie folders inside letter, genre and collection folders are found; a
// folder that only groups other folders is never offered as a movie.
func TestPlanAdoptFindsNestedMovieFolders(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	nestedTree(t, lf.Path)
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A-Z/D/Dune - Part Two (2024)", "Action/Heat (1995)", "Empty Movie (2001)",
		"Lantern Collection/The Lantern Keepers (2027)", "Lantern Collection/The Lantern Keepers Return (2029)", "Lone Disc"}
	if got := folderNames(p); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("folders %q\nwant %q", got, want)
	}
	got := byFolder(p)
	if c := got["Action/Heat (1995)"]; c.Status != "matched" || c.Match == nil || c.Match.TMDB != "949" || c.Main != "Action/Heat (1995)/heat.avi" {
		t.Fatalf("heat %+v", c)
	}
	if c := got["A-Z/D/Dune - Part Two (2024)"]; c.Main != "A-Z/D/Dune - Part Two (2024)/dune.mkv" {
		t.Fatalf("dune %+v", c)
	}
	if p.Total != 6 {
		t.Fatalf("total %d", p.Total)
	}
}

// The scan limit counts movie folders wherever they sit, and the next
// part goes on after the last one.
func TestPlanAdoptNestedIsCappedAndGoesOn(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for i := range 7 {
		writeFile(t, filepath.Join(lf.Path, "Action", fmt.Sprintf("Movie %02d (2001)", i), "m.mkv"), "m")
		writeFile(t, filepath.Join(lf.Path, "Drama", fmt.Sprintf("Movie %02d (2002)", i), "m.mkv"), "m")
	}
	ns := mediafiles.NamingDefaults()
	ns.AdoptScanLimit = 10
	if err := mediafiles.PutNamingSettings(context.Background(), s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	p, err := PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, "")
	if err != nil || len(p.Folders) != 10 || !p.More || p.Next != "Drama/Movie 02 (2002)" || p.Total != 14 || p.Offset != 0 {
		t.Fatalf("first %d more=%v next=%q total=%d %v", len(p.Folders), p.More, p.Next, p.Total, err)
	}
	p, err = PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, p.Next)
	if err != nil || len(p.Folders) != 4 || p.More || p.Folders[0].Folder != "Drama/Movie 03 (2002)" || p.Offset != 10 {
		t.Fatalf("rest %+v %v", p, err)
	}
}

// A nested movie folder is adopted where it is, and is in the Library on
// the next scan; paths that leave the Library folder are refused.
func TestAdoptNestedFolder(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	nestedTree(t, lf.Path)
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	ctx := context.Background()
	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Action/Heat (1995)", TMDB: "949"},
		{Folder: "Action/../../outside", TMDB: "949"}, {Folder: "Lantern Collection/.hidden", TMDB: "900001"}, {Folder: `Action\Heat (1995)`, TMDB: "949"}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].TitleID == 0 || res[0].File != "Action/Heat (1995)/heat.avi" || res[1].Error == "" || res[2].Error == "" || res[3].Error == "" {
		t.Fatalf("results %+v", res)
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", res[0].TitleID)
	if len(vs) != 1 || vs[0].FolderName != "Action/Heat (1995)" {
		t.Fatalf("version %+v", vs)
	}
	p, _ := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if c := byFolder(p)["Action/Heat (1995)"]; c.Status != "in_library" {
		t.Fatalf("rescan %+v", c)
	}
	if strings.Contains(fmt.Sprint(folderNames(p)), "\"Action\"") {
		t.Fatal("grouping folder offered")
	}
}

// A yearless movie whose videos sit only in disc, bonus or feature
// folders stays one movie; a yearless folder of yearless movies groups.
func TestPlanAdoptKeepsYearlessMovieWhole(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Heat", "Disc 1 - Feature", "heat.mkv"), "feature")
	writeFile(t, filepath.Join(lf.Path, "Heat", "Disc 2 - Bonus", "bonus.mkv"), "b")
	writeFile(t, filepath.Join(lf.Path, "Brazil", "Making Of", "mo.mkv"), "m")
	writeFile(t, filepath.Join(lf.Path, "Brazil", "Bonus", "b.mkv"), "b")
	writeFile(t, filepath.Join(lf.Path, "Alien", "Feature", "alien.mkv"), "a")
	writeFile(t, filepath.Join(lf.Path, "Rocky", "Specials", "s.mkv"), "s")
	writeFile(t, filepath.Join(lf.Path, "Rocky", "Extras", "e.mkv"), "e")
	writeFile(t, filepath.Join(lf.Path, "Rocky", "Deleted Scenes", "d.mkv"), "d")
	writeFile(t, filepath.Join(lf.Path, "Kids", "Lantern Keepers", "lk.mkv"), "l")
	writeFile(t, filepath.Join(lf.Path, "Kids", "Moon Garden", "mg.mkv"), "m")
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Alien", "Brazil", "Heat", "Kids/Lantern Keepers", "Kids/Moon Garden", "Rocky"}
	if got := folderNames(p); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("folders %q\nwant %q", got, want)
	}
}

// A Version's own folder may sit inside grouping folders, with each part
// checked as a folder name.
func TestCustomVersionFolder(t *testing.T) {
	for in, ok := range map[string]bool{"Heat (1995)": true, "Action/Heat (1995)": true, "A-Z/H/Heat": true,
		"../Heat": false, "Action/../Heat": false, "Action/.Heat": false, `Action\Heat`: false, "/Heat": false, "Action/": false,
		"Action/He:at": false, "Action/CON": false, "Heat.": false, "": false} {
		if got := customFolderOK(in); got != ok {
			t.Errorf("%q: %v, want %v", in, got, ok)
		}
	}
}

// Movies whose names start like extras folders are still movies, and a
// year or TMDB tag always makes a folder a movie.
func TestPlanAdoptMovieNamesLikeExtras(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Comedy", "Short Circuit (1986)", "sc.mkv"), "s")
	writeFile(t, filepath.Join(lf.Path, "Comedy", "Extra Ordinary (2019)", "eo.mkv"), "e")
	writeFile(t, filepath.Join(lf.Path, "Kids", "Shorts", "s.mkv"), "s")
	writeFile(t, filepath.Join(lf.Path, "Kids", "Other People", "o.mkv"), "o")
	writeFile(t, filepath.Join(lf.Path, "Extraction Collection", "Extraction (2020)", "x.mkv"), "x")
	writeFile(t, filepath.Join(lf.Path, "Extraction Collection", "Extraction 2 (2023)", "x2.mkv"), "x")
	writeFile(t, filepath.Join(lf.Path, "Music Films", "Disco (2008)", "d.mkv"), "d")
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Comedy/Extra Ordinary (2019)", "Comedy/Short Circuit (1986)", "Extraction Collection/Extraction (2020)",
		"Extraction Collection/Extraction 2 (2023)", "Kids/Other People", "Kids/Shorts", "Music Films/Disco (2008)"}
	if got := folderNames(p); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("folders %q\nwant %q", got, want)
	}
}

func TestPartFolderWholeNamesOnly(t *testing.T) {
	for in, want := range map[string]bool{"Disc 1": true, "cd2": true, "Part 2": true, "Disc 1 - Feature": true, "DVD.A": true, "Extras": true,
		"Bonus Features": true, "Making Of": true, "Specials": true, "Subs": true, "VIDEO_TS": true, "Feature": true,
		"Disco": false, "Extraction": false, "Special Ops": false, "Short Circuit": false, "Shorts": false, "Sample People": false,
		"Substitute": false, "Scenes from a Marriage": false, "Trailer Park Boys": false, "Other People": false, "Partners": false} {
		if got := partFolder(in); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
	for _, f := range []string{" Action/Heat", "Action /Heat", "Action/Heat "} {
		if validFolder(f) || customFolderOK(f) {
			t.Errorf("%q accepted", f)
		}
	}
}

// Dotted extras names are extras, a grouping folder's own Extras folder is
// not a movie, and "Part B Movie" is a movie name, not a disc.
func TestPlanAdoptExtrasEdgeNames(t *testing.T) {
	for in, want := range map[string]bool{"making.of": true, "behind.the.scenes": true, "Deleted_Scenes": true, "VIDEO_TS": true,
		"Part B": true, "Disc.A": true, "Part B Movie": false, "Disc 1 - Feature": true} {
		if got := partFolder(in); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Film", "film.mkv"), "f")
	writeFile(t, filepath.Join(lf.Path, "Film2", "making.of", "m.mkv"), "m")
	writeFile(t, filepath.Join(lf.Path, "Film2", "behind.the.scenes", "b.mkv"), "b")
	writeFile(t, filepath.Join(lf.Path, "Kids", "Alpha", "a.mkv"), "a")
	writeFile(t, filepath.Join(lf.Path, "Kids", "Beta", "b.mkv"), "b")
	writeFile(t, filepath.Join(lf.Path, "Kids", "Extras", "e.mkv"), "e")
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Film", "Film2", "Kids/Alpha", "Kids/Beta"}
	if got := folderNames(p); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("folders %q\nwant %q", got, want)
	}
}

// Paging a nested library reads every movie folder exactly once.
func TestPlanAdoptNestedPagesNoSkipNoDup(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	var want []string
	for i := range 6 {
		for _, g := range []string{"A-Z/M", "Action", "Drama Collection"} {
			f := fmt.Sprintf("%s/Movie %02d (2001)", g, i)
			writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(f), "m.mkv"), "m")
			want = append(want, f)
		}
		f := fmt.Sprintf("Top %02d (2003)", i)
		writeFile(t, filepath.Join(lf.Path, f, "m.mkv"), "m")
		want = append(want, f)
	}
	sort.Strings(want)
	ns := mediafiles.NamingDefaults()
	ns.AdoptScanLimit = 10
	if err := mediafiles.PutNamingSettings(context.Background(), s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	var got []string
	after := ""
	for range 10 {
		p, err := PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, after)
		if err != nil || p.Offset != len(got) || p.Total != len(want) {
			t.Fatalf("part after %q: offset %d total %d %v", after, p.Offset, p.Total, err)
		}
		for _, c := range p.Folders {
			got = append(got, c.Folder)
		}
		if !p.More {
			break
		}
		after = p.Next
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("paged %q\nwant %q", got, want)
	}
}
