package library

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func groupByGenre(t *testing.T, s *database.Store, on bool) {
	t.Helper()
	ns, err := mediafiles.ReadNaming(context.Background(), s.Read())
	if err != nil {
		t.Fatal(err)
	}
	ns.GroupByGenre = on
	if err := mediafiles.PutNamingSettings(context.Background(), s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
}

// Group by genre is off by default; on, a movie folder sits in a folder of
// its first genre, and a movie with no genre stays at the top.
func TestMovieFolderGroupByGenre(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	f := mediafiles.NameFacts{Title: "Heat", Year: 1995, Genre: "Crime"}
	if ns.GroupByGenre || mediafiles.MovieFolder(ns, f) != "Heat (1995)" {
		t.Fatalf("default %v %q", ns.GroupByGenre, mediafiles.MovieFolder(ns, f))
	}
	ns.GroupByGenre = true
	if got := mediafiles.MovieFolder(ns, f); got != "Crime/Heat (1995)" {
		t.Fatal(got)
	}
	if got := mediafiles.MovieFolder(ns, mediafiles.NameFacts{Title: "Heat", Year: 1995, Genre: "Action/Adventure"}); filepath.Dir(filepath.FromSlash(got)) == "Action" {
		t.Fatalf("a slash in a genre made two folders: %q", got)
	}
	if got := mediafiles.MovieFolder(ns, mediafiles.NameFacts{Title: "Heat", Year: 1995}); got != "Heat (1995)" {
		t.Fatal(got)
	}
	s := open(t)
	groupByGenre(t, s, true)
	if back, _ := mediafiles.ReadNaming(context.Background(), s.Read()); !back.GroupByGenre {
		t.Fatal("not saved")
	}
}

// A movie added with Group by genre on gets Genre/Title (Year)/.
func TestAddMovieGroupByGenre(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	groupByGenre(t, s, true)
	m := dune
	m.Genres = []string{"Science Fiction", "Adventure"}
	id, err := AddMovie(context.Background(), s, m, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ := database.Select[mediafiles.Version](context.Background(), s.Read(), "versions", "title_id = ?", id)
	if len(vs) != 1 || vs[0].FolderName != "Science Fiction/Dune - Part Two (2024)" || vs[0].LibraryFolderID != lf.ID {
		t.Fatalf("versions %+v", vs)
	}
}

// With the option on, a rename moves a movie folder into its genre folder
// with its subtitle, and the Version's folder follows.
func TestRenameGroupByGenre(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	m := dune
	m.Genres = []string{"Science Fiction"}
	id, err := AddMovie(context.Background(), s, m, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: s, Now: func() time.Time { return now }}
	ctx := context.Background()
	putFile(t, s, lf, id, "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv", "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS", "imported")
	writeFile(t, filepath.Join(lf.Path, "Dune - Part Two (2024)", "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.en.srt"), "sub")
	groupByGenre(t, s, true)
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 {
		t.Fatalf("%+v %v", plan, err)
	}
	if d := filepath.ToSlash(filepath.Dir(plan[0].To)); d != "Science Fiction/Dune - Part Two (2024)" {
		t.Fatalf("to %q", plan[0].To)
	}
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	vs, _ := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ?", id)
	if vs[0].FolderName != "Science Fiction/Dune - Part Two (2024)" {
		t.Fatalf("version folder %q", vs[0].FolderName)
	}
	if !exists(filepath.Join(lf.Path, "Science Fiction", "Dune - Part Two (2024)", "Dune - Part Two (2024) 1080p BluRay.en.srt")) && !exists(filepath.Join(lf.Path, "Science Fiction", "Dune - Part Two (2024)")) {
		t.Fatal("not moved")
	}
	if exists(filepath.Join(lf.Path, "Dune - Part Two (2024)")) {
		t.Fatal("old folder left behind")
	}
	// Moved once: the next preview has nothing to do.
	if again, _ := PlanRename(ctx, s.Read(), []int64{id}); len(again) != 0 {
		t.Fatalf("again %+v", again)
	}
}

// A movie folder that holds other files (extras) is never moved by a
// rename, so nothing is left behind.
func TestRenameGroupByGenreKeepsFoldersWithExtras(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	m := dune
	m.Genres = []string{"Science Fiction"}
	id, _ := AddMovie(context.Background(), s, m, AddOptions{}, nil, now, domain.System)
	putFile(t, s, lf, id, "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv", "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS", "imported")
	writeFile(t, filepath.Join(lf.Path, "Dune - Part Two (2024)", "Extras", "making of.mkv"), "x")
	groupByGenre(t, s, true)
	plan, _ := PlanRename(context.Background(), s.Read(), []int64{id})
	if len(plan) != 1 || filepath.ToSlash(filepath.Dir(plan[0].To)) != "Dune - Part Two (2024)" {
		t.Fatalf("%+v", plan)
	}
}

// Import a library already reads Genre/Title (Year)/ folders as movies.
func TestPlanAdoptGenreLayout(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Crime", "Heat (1995)", "heat.mkv"), "h")
	writeFile(t, filepath.Join(lf.Path, "Science Fiction", "Alien (1979)", "alien.mkv"), "a")
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := folderNames(p)
	if len(got) != 2 || got[0] != "Crime/Heat (1995)" || got[1] != "Science Fiction/Alien (1979)" {
		t.Fatalf("%q", got)
	}
}

// Tidy keeps the person's genre folders whether Group by genre is on or
// off; movies loose at the top get their folder at the top.
func TestTidyGroupByGenre(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Horror", "Alien (1979).mkv"), "a")
	writeFile(t, filepath.Join(lf.Path, "Horror", "Halloween (1978).mkv"), "h")
	writeFile(t, filepath.Join(lf.Path, "Heat (1995).mkv"), "1")
	writeFile(t, filepath.Join(lf.Path, "Ronin (1998).mkv"), "2")
	ctx := context.Background()
	tos := func() map[string]string {
		p, _ := PlanTidy(ctx, s.Read(), lf.ID)
		out := map[string]string{}
		for _, m := range p.Moves {
			out[m.From] = m.To
		}
		return out
	}
	if got := tos(); got["Horror/Alien (1979).mkv"] != "Horror/Alien (1979)/Alien (1979).mkv" || got["Heat (1995).mkv"] != "Heat (1995)/Heat (1995).mkv" {
		t.Fatalf("off %v", got)
	}
	groupByGenre(t, s, true)
	got := tos()
	if got["Horror/Alien (1979).mkv"] != "Horror/Alien (1979)/Alien (1979).mkv" || got["Heat (1995).mkv"] != "Heat (1995)/Heat (1995).mkv" {
		t.Fatalf("on %v", got)
	}
	p, _ := PlanTidy(ctx, s.Read(), lf.ID)
	if _, err := ApplyTidy(ctx, s, lf.ID, p.Hash, domain.System); err != nil {
		t.Fatal(err)
	}
	if ap, _ := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID); ap.LooseMovies != 0 || len(ap.Folders) != 4 {
		t.Fatalf("%v %d", folderNames(ap), ap.LooseMovies)
	}
}
