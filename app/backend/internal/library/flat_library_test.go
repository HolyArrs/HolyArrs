package library

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// a movie Library folder with no folders at all, every movie a file
// directly in it, is a normal way to keep movies.

var flatTMDB = &metadata.Static{Movies: []metadata.Movie{
	{Name: "Alien", Year: 1979, IDs: map[string]string{"tmdb": "348"}, DigitalDate: d("1980-01-01")},
	{Name: "Heat", Year: 1995, IDs: map[string]string{"tmdb": "949"}, DigitalDate: d("1996-01-01")},
	{Name: "Blade Runner", Year: 1982, IDs: map[string]string{"tmdb": "78"}, DigitalDate: d("1983-01-01")},
	{Name: "Dune", Year: 2021, IDs: map[string]string{"tmdb": "438631"}, DigitalDate: d("2022-01-01")},
	{Name: "Arrival", Year: 2016, IDs: map[string]string{"tmdb": "329865"}, DigitalDate: d("2017-01-01")},
}}

// flatTree is the test library of the plan.
func flatTree(t *testing.T, lib string) {
	t.Helper()
	writeFile(t, filepath.Join(lib, "Alien (1979).mkv"), strings.Repeat("a", 40))
	writeFile(t, filepath.Join(lib, "Alien (1979).en.srt"), "1\n00:00:01,000 --> 00:00:02,000\nhi\n")
	writeFile(t, filepath.Join(lib, "Alien (1979).nfo"), "<movie/>")
	writeFile(t, filepath.Join(lib, "Heat.1995.1080p.BluRay.x264.mkv"), strings.Repeat("h", 50))
	writeFile(t, filepath.Join(lib, "Heat.1995.1080p.BluRay.x264.forced.en.srt"), "1\n00:00:01,000 --> 00:00:02,000\nho\n")
	writeFile(t, filepath.Join(lib, "Blade Runner (1982) - Final Cut.mkv"), strings.Repeat("b", 60))
	writeFile(t, filepath.Join(lib, "Blade Runner (1982) - Theatrical.mkv"), strings.Repeat("t", 30))
	writeFile(t, filepath.Join(lib, "poster.jpg"), "jpg")
	writeFile(t, filepath.Join(lib, "notes.txt"), "notes")
	writeFile(t, filepath.Join(lib, "Dune (2021)", "Dune (2021).mkv"), strings.Repeat("d", 70))
	writeFile(t, filepath.Join(lib, "Random Home Video.mp4"), "r")
}

func flatFolder(t *testing.T, s *database.Store, flat bool) mediafiles.LibraryFolder {
	t.Helper()
	lf := addFolder(t, s)
	flatTree(t, lf.Path)
	if flat {
		on := true
		var err error
		if lf, err = UpdateFolder(context.Background(), s, lf.ID, FolderChange{Flat: &on}, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	return lf
}

var flatChoices = []AdoptChoice{{Folder: "Alien (1979).mkv", TMDB: "348"}, {Folder: "Heat.1995.1080p.BluRay.x264.mkv", TMDB: "949"},
	{Folder: "Blade Runner (1982) - Final Cut.mkv", TMDB: "78"}, {Folder: "Dune (2021)", TMDB: "438631"}}

// adoptFlat adopts the four movies of flatTree and gives their title ids by name.
func adoptFlat(t *testing.T, svc *Service, lf mediafiles.LibraryFolder) map[string]int64 {
	t.Helper()
	res, err := svc.Adopt(context.Background(), lf.ID, flatChoices, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for i, r := range res {
		if r.Error != "" || r.TitleID == 0 {
			t.Fatalf("adopt %s: %+v", flatChoices[i].Folder, r)
		}
		ids[strings.SplitN(flatChoices[i].Folder, " ", 2)[0]] = r.TitleID
	}
	ids["Heat"] = ids["Heat.1995.1080p.BluRay.x264.mkv"]
	return ids
}

func flatService(s *database.Store) *Service {
	return &Service{Store: s, Metadata: flatTMDB, Now: func() time.Time { return now }}
}

// sameExcept fails when after differs from before in anything but the
// paths named in gone (left) and added (new).
func sameExcept(t *testing.T, before, after map[string]string, gone, added []string) {
	t.Helper()
	want := maps.Clone(before)
	for _, g := range gone {
		if _, ok := want[g]; !ok {
			t.Fatalf("%s was never there", g)
		}
		delete(want, g)
	}
	for _, a := range added {
		if _, ok := after[a]; !ok {
			t.Fatalf("%s missing after; have %v", a, slices.Sorted(maps.Keys(after)))
		}
		want[a] = after[a]
	}
	if !maps.Equal(want, after) {
		t.Fatalf("disk changed:\nwant %v\n got %v", slices.Sorted(maps.Keys(want)), slices.Sorted(maps.Keys(after)))
	}
}

// Scenarios 1-4: a flat folder is scanned file by file; nothing moves.
func TestFlatScanOffersLooseMovies(t *testing.T) {
	for _, flat := range []bool{false, true} {
		t.Run(fmt.Sprint("flat=", flat), func(t *testing.T) {
			s := open(t)
			lf := flatFolder(t, s, flat)
			before := snapshot(t, lf.Path)
			p, err := PlanAdopt(context.Background(), s.Read(), flatTMDB, lf.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"Alien (1979).mkv", "Blade Runner (1982) - Final Cut.mkv", "Dune (2021)", "Heat.1995.1080p.BluRay.x264.mkv", "Random Home Video.mp4"}
			if got := folderNames(p); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("candidates %q\nwant %q", got, want)
			}
			c := byFolder(p)
			for _, n := range []string{"Alien (1979).mkv", "Heat.1995.1080p.BluRay.x264.mkv", "Blade Runner (1982) - Final Cut.mkv", "Dune (2021)"} {
				if c[n].Status != "matched" {
					t.Fatalf("%s: %+v", n, c[n])
				}
			}
			if c["Heat.1995.1080p.BluRay.x264.mkv"].Match.TMDB != "949" || !c["Heat.1995.1080p.BluRay.x264.mkv"].Loose || c["Dune (2021)"].Loose {
				t.Fatalf("heat %+v dune %+v", c["Heat.1995.1080p.BluRay.x264.mkv"], c["Dune (2021)"])
			}
			// Two cuts of one movie are one movie; the bigger one is its file.
			br := c["Blade Runner (1982) - Final Cut.mkv"]
			if len(br.Videos) != 2 || br.Main != "Blade Runner (1982) - Final Cut.mkv" {
				t.Fatalf("blade runner %+v", br)
			}
			if r := c["Random Home Video.mp4"]; r.Status == "matched" {
				t.Fatalf("a home video matched: %+v", r)
			}
			// Flat is suggested only when not set; Tidy is an option then, never a must.
			if p.Flat != flat || p.SuggestFlat == flat || p.LooseMovies != 0 {
				t.Fatalf("flat %v suggest %v left out %d", p.Flat, p.SuggestFlat, p.LooseMovies)
			}
			if wantTop := map[bool]int{false: 3, true: 0}[flat]; p.LooseTop != wantTop {
				t.Fatalf("loose top %d, want %d", p.LooseTop, wantTop)
			}
			sameExcept(t, before, snapshot(t, lf.Path), nil, nil)
		})
	}
}

// Scenarios 2, 5, 6: adopting keeps every file where it is; each movie is
// its own file, with no folder of its own; a scan again knows them.
func TestFlatAdoptKeepsFilesInPlace(t *testing.T) {
	s := open(t)
	lf := flatFolder(t, s, true)
	before := snapshot(t, lf.Path)
	svc := flatService(s)
	ids := adoptFlat(t, svc, lf)
	sameExcept(t, before, snapshot(t, lf.Path), nil, nil)
	ctx := context.Background()
	want := map[string][2]string{"Alien": {"", "Alien (1979).mkv"}, "Heat": {"", "Heat.1995.1080p.BluRay.x264.mkv"},
		"Blade": {"", "Blade Runner (1982) - Final Cut.mkv"}, "Dune": {"Dune (2021)", "Dune (2021)/Dune (2021).mkv"}}
	for name, w := range want {
		vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", ids[name])
		fs, _ := database.Select[mediafiles.File](ctx, s.Read(), "files", "version_id = ? AND state = 'present'", vs[0].ID)
		if len(vs) != 1 || vs[0].FolderName != w[0] || len(fs) != 1 || fs[0].RelativePath != w[1] {
			t.Fatalf("%s: versions %+v files %+v", name, vs, fs)
		}
	}
	p, err := PlanAdopt(ctx, s.Read(), flatTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Folders {
		if c.Folder != "Random Home Video.mp4" && c.Status != "in_library" {
			t.Fatalf("scan again: %+v", c)
		}
	}
	// Adopting again (a run started twice) adds nothing new.
	res, err := svc.Adopt(ctx, lf.ID, flatChoices[:1], domain.System)
	if err != nil || res[0].TitleID != ids["Alien"] {
		t.Fatalf("again %+v %v", res, err)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM titles").Scan(&n)
	if n != 4 {
		t.Fatalf("%d titles", n)
	}
	// A choice that leaves the folder or is not a video is refused.
	for _, bad := range []string{"../x.mkv", "notes.txt", "poster.jpg", "missing (2000).mkv"} {
		if res, _ := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: bad, TMDB: "348"}}, domain.System); res[0].Error == "" {
			t.Fatalf("%s adopted: %+v", bad, res)
		}
	}
	sameExcept(t, before, snapshot(t, lf.Path), nil, nil)
}

// Scenarios 11-14: deleting one movie of a flat folder takes only its own
// files; the Library folder and every other file stay.
func TestFlatDeleteTakesOnlyThatMovie(t *testing.T) {
	s := open(t)
	lf := flatFolder(t, s, true)
	svc := flatService(s)
	ids := adoptFlat(t, svc, lf)
	ctx := context.Background()
	before := snapshot(t, lf.Path)
	// Keep files: nothing on disk changes.
	if _, err := svc.Remove(ctx, ids["Heat"], RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	sameExcept(t, before, snapshot(t, lf.Path), nil, nil)
	plan, err := svc.Remove(ctx, ids["Alien"], RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Held) != 1 || plan.Held[0].Path != "Alien (1979).mkv" {
		t.Fatalf("held %+v", plan.Held)
	}
	after := snapshot(t, lf.Path)
	var added []string
	for p := range after {
		if _, ok := before[p]; !ok {
			added = append(added, p)
		}
	}
	slices.Sort(added)
	if len(added) != 2 || !strings.HasSuffix(added[0], "/Alien (1979).en.srt") || !strings.HasSuffix(added[1], "/Alien (1979).mkv") {
		t.Fatalf("held to %v", added)
	}
	sameExcept(t, before, after, []string{"Alien (1979).mkv", "Alien (1979).en.srt"}, added)
	// One cut of Blade Runner goes, the other stays; then the last movies
	// go and the Library folder itself is still there.
	for _, name := range []string{"Blade", "Dune"} {
		if _, err := svc.Remove(ctx, ids[name], RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	for _, keep := range []string{"Blade Runner (1982) - Theatrical.mkv", "Heat.1995.1080p.BluRay.x264.mkv", "poster.jpg", "notes.txt", "Alien (1979).nfo", "Random Home Video.mp4"} {
		if !onDisk(lf, keep) {
			t.Fatalf("%s is gone", keep)
		}
	}
	if onDisk(lf, "Blade Runner (1982) - Final Cut.mkv") || onDisk(lf, "Dune (2021)") {
		t.Fatal("deleted files still at their place")
	}
	if fi, err := os.Stat(lf.Path); err != nil || !fi.IsDir() {
		t.Fatal("the Library folder went")
	}
}

// The last movie of a flat folder going leaves the empty Library folder.
func TestFlatDeleteLastMovieKeepsTheFolder(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Alien (1979).mkv"), "a")
	svc := flatService(s)
	res, _ := svc.Adopt(context.Background(), lf.ID, flatChoices[:1], domain.System)
	if _, err := svc.Remove(context.Background(), res[0].TitleID, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(lf.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), ".") {
			t.Fatalf("left %s", e.Name())
		}
	}
}

// Scenario 9: renaming in a flat folder renames only the movies' own files
// and their subtitles, at the top; no folder is made or renamed.
func TestFlatRenameStaysFlat(t *testing.T) {
	s := open(t)
	lf := flatFolder(t, s, true)
	svc := flatService(s)
	ids := adoptFlat(t, svc, lf)
	ctx := context.Background()
	before := snapshot(t, lf.Path)
	plan, err := PlanRename(ctx, s.Read(), []int64{ids["Alien"], ids["Heat"], ids["Blade"], ids["Dune"]})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range plan {
		if st.Skip == "" && strings.Contains(st.To, "/") != strings.Contains(st.From, "/") {
			t.Fatalf("a rename changes folders: %+v", st)
		}
	}
	if _, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, lf.Path)
	for _, keep := range []string{"poster.jpg", "notes.txt", "Random Home Video.mp4", "Blade Runner (1982) - Theatrical.mkv", "Alien (1979).nfo"} {
		if before[keep] == "" || after[keep] != before[keep] {
			t.Fatalf("%s changed", keep)
		}
	}
	if len(after) != len(before) {
		t.Fatalf("files %d, were %d", len(after), len(before))
	}
	ents, _ := os.ReadDir(lf.Path)
	var dirs []string
	for _, e := range ents {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if fmt.Sprint(dirs) != "[Dune (2021)]" {
		t.Fatalf("folders %v", dirs)
	}
	f, _ := database.Select[mediafiles.File](ctx, s.Read(), "files", "version_id IN (SELECT id FROM versions WHERE title_id = ?) AND state = 'present'", ids["Heat"])
	if len(f) != 1 || strings.Contains(f[0].RelativePath, "/") || !onDisk(lf, f[0].RelativePath) {
		t.Fatalf("heat %+v", f)
	}
}

// Scenario 17: Group by genre never moves a movie kept flat.
func TestFlatGenreMoveLeavesLooseFiles(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.GroupByGenre = true
	v := mediafiles.Version{FolderName: ""}
	tt := mediafiles.Title{Name: "Alien", Genres: []string{"Horror"}}
	if to := genreMove(ns, tt, v, t.TempDir(), "Alien (1979).mkv", "Alien (1979) 1080p.mkv"); to != "Alien (1979) 1080p.mkv" {
		t.Fatalf("to %q", to)
	}
}

// Scenario 7, 15: a new movie or a second copy in a flat folder has no
// folder of its own; outside one it gets a folder as before.
func TestFlatNewMovieAndCopyHaveNoFolder(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := flatFolder(t, s, true)
	arrival := flatTMDB.Movies[4]
	id, err := AddMovie(ctx, s, arrival, AddOptions{LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", id)
	if len(vs) != 1 || vs[0].FolderName != "" {
		t.Fatalf("versions %+v", vs)
	}
	prID := vs[0].PickRulesID
	v2, err := AddVersion(ctx, s, id, VersionInput{Label: "4K", PickRulesID: prID, LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil || v2.FolderName != "" {
		t.Fatalf("4K copy %+v %v", v2, err)
	}
	other := addFolder(t, s)
	id2, err := AddMovie(ctx, s, flatTMDB.Movies[0], AddOptions{LibraryFolderID: other.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ = database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", id2)
	if vs[0].FolderName == "" {
		t.Fatal("a folder that is not flat got a loose movie")
	}
	// A loose movie in a folder that is not flat: a second copy there gets a folder.
	notFlat := flatFolder(t, s, false)
	svc := flatService(s)
	res, _ := svc.Adopt(ctx, notFlat.ID, []AdoptChoice{{Folder: "Heat.1995.1080p.BluRay.x264.mkv", TMDB: "949"}}, domain.System)
	v3, err := AddVersion(ctx, s, res[0].TitleID, VersionInput{Label: "4K", PickRulesID: prID, LibraryFolderID: notFlat.ID}, nil, now, domain.System)
	if err != nil || v3.FolderName == "" || strings.HasPrefix(v3.FolderName, " ") {
		t.Fatalf("copy next to a loose file %+v %v", v3, err)
	}
}

// Scenarios 16, 23: Tidy my folders never touches a flat folder's top, nor
// a movie already in the Library; switching modes moves nothing.
func TestFlatTidy(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := flatFolder(t, s, true)
	before := snapshot(t, lf.Path)
	p, err := PlanTidy(ctx, s.Read(), lf.ID)
	if err != nil || len(p.Moves) != 0 {
		t.Fatalf("flat tidy %+v %v", p, err)
	}
	off := false
	if _, err := UpdateFolder(ctx, s, lf.ID, FolderChange{Flat: &off}, domain.System); err != nil {
		t.Fatal(err)
	}
	sameExcept(t, before, snapshot(t, lf.Path), nil, nil)
	svc := flatService(s)
	if res, _ := svc.Adopt(ctx, lf.ID, flatChoices[:1], domain.System); res[0].Error != "" {
		t.Fatal(res[0].Error)
	}
	p, err = PlanTidy(ctx, s.Read(), lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	var from []string
	for _, m := range p.Moves {
		from = append(from, m.From)
	}
	slices.Sort(from)
	if fmt.Sprint(from) != "[Blade Runner (1982) - Final Cut.mkv Blade Runner (1982) - Theatrical.mkv Heat.1995.1080p.BluRay.x264.mkv]" {
		t.Fatalf("tidy moves %v", from)
	}
	on := true
	if _, err := UpdateFolder(ctx, s, lf.ID, FolderChange{Flat: &on}, domain.System); err != nil {
		t.Fatal(err)
	}
	sameExcept(t, before, snapshot(t, lf.Path), nil, nil)
	// Flat is a movie setting only.
	shows, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "show", Label: "Shows", Flat: true}, nil, domain.System)
	if err != nil || shows.Flat {
		t.Fatalf("show folder flat %v %v", shows.Flat, err)
	}
}

// Scenarios 21, 22: moving a movie between a flat folder and one that is
// not moves only its own files.
func TestFlatMoveBetweenFolders(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	flat := flatFolder(t, s, true)
	ids := adoptFlat(t, flatService(s), flat)
	foldered := addFolder(t, s)
	before := snapshot(t, flat.Path)
	plans, err := PlanMove(ctx, s.Read(), s.Read(), ids["Alien"], foldered.ID)
	if err != nil || len(plans) != 1 || plans[0].Blocked != "" || len(plans[0].Steps) != 1 || plans[0].ToFolderName != "Alien (1979)" {
		t.Fatalf("plan %+v %v", plans, err)
	}
	svc, _, _ := runQueue(t, s, nil)
	svc.Metadata = flatTMDB
	if _, err := svc.Move(ctx, ids["Alien"], foldered.ID, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if j := waitJobs(t, s, 1); j[0].State != "done" {
		t.Fatalf("move %+v", j)
	}
	sameExcept(t, before, snapshot(t, flat.Path), []string{"Alien (1979).mkv"}, nil)
	if !onDisk(foldered, "Alien (1979)/Alien (1979).mkv") {
		t.Fatal("not in its new folder")
	}
	var folder string
	_ = s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ?", ids["Alien"]).Scan(&folder)
	if folder != "Alien (1979)" {
		t.Fatalf("folder %q", folder)
	}
	// Back into the flat folder: the file goes to its top, the empty movie folder goes.
	if _, err := svc.Move(ctx, ids["Alien"], flat.ID, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if j := waitJobs(t, s, 2); j[len(j)-1].State != "done" {
		t.Fatalf("move back %+v %v", j, *j[len(j)-1].Error)
	}
	sameExcept(t, before, snapshot(t, flat.Path), nil, nil)
	if _, err := os.Stat(filepath.Join(foldered.Path, "Alien (1979)")); !os.IsNotExist(err) {
		t.Fatal("the empty movie folder stayed")
	}
	_ = s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ?", ids["Alien"]).Scan(&folder)
	if folder != "" {
		t.Fatalf("folder %q", folder)
	}
	// A movie folder with a file in a subfolder cannot go flat.

	hid, err := AddMovie(ctx, s, flatTMDB.Movies[4], AddOptions{LibraryFolderID: foldered.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, s, foldered, hid, "Arrival (2016)/Extras/x.mkv", "x", "adopted")
	if plans, _ := PlanMove(ctx, s.Read(), s.Read(), hid, flat.ID); len(plans) != 1 || plans[0].Blocked == "" {
		t.Fatalf("plan %+v", plans)
	}
}

// Versions: a file of another movie in a flat folder cannot be added as a copy.
func TestFlatCopyChoiceRefusesOtherMovie(t *testing.T) {
	s := open(t)
	lf := flatFolder(t, s, true)
	svc := flatService(s)
	ids := adoptFlat(t, svc, lf)
	ctx := context.Background()
	var prID int64
	_ = s.Read().QueryRow("SELECT pick_rules_id FROM versions WHERE title_id = ?", ids["Alien"]).Scan(&prID)
	v4k, err := AddVersion(ctx, s, ids["Alien"], VersionInput{Label: "4K", PickRulesID: prID, LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdoptMovieCopy(ctx, ids["Alien"], v4k.ID, CopyAdoption{RelativePath: "Heat.1995.1080p.BluRay.x264.mkv"}, domain.System); err == nil {
		t.Fatal("another movie's file became a copy")
	}
	writeFile(t, filepath.Join(lf.Path, "Alien (1979) 2160p.mkv"), "4k")
	if _, err := svc.AdoptMovieCopy(ctx, ids["Alien"], v4k.ID, CopyAdoption{RelativePath: "Alien (1979) 2160p.mkv"}, domain.System); err != nil {
		t.Fatalf("own 4K file: %v", err)
	}
}

// Moving a flat movie into a folder that already holds another Version of it
// under the movie folder name would put two Versions in one folder: refused.
func TestFlatMoveRefusesFolderNameAnotherVersionUses(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	flat := flatFolder(t, s, true)
	ids := adoptFlat(t, flatService(s), flat)
	foldered := addFolder(t, s)
	var prID int64
	_ = s.Read().QueryRow("SELECT pick_rules_id FROM versions WHERE title_id = ?", ids["Alien"]).Scan(&prID)
	other, err := AddVersion(ctx, s, ids["Alien"], VersionInput{Label: "4K", PickRulesID: prID, LibraryFolderID: foldered.ID}, nil, now, domain.System)
	if err != nil || other.FolderName != "Alien (1979)" {
		t.Fatalf("other version %+v %v", other, err)
	}
	plans, err := PlanMove(ctx, s.Read(), s.Read(), ids["Alien"], foldered.ID)
	if err != nil || len(plans) != 1 || plans[0].Blocked == "" || len(plans[0].Steps) != 0 {
		t.Fatalf("plan %+v %v", plans, err)
	}
	// Another folder name for the other Version frees the way.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE versions SET folder_name = ? WHERE id = ?", "Alien (1979) [4K]", other.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if plans, err = PlanMove(ctx, s.Read(), s.Read(), ids["Alien"], foldered.ID); err != nil || len(plans) != 1 || plans[0].Blocked != "" {
		t.Fatalf("plan %+v %v", plans, err)
	}
}
