package library

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// Genre folders that hold movies as loose files, or loose files next
// to movie folders, are never offered as one movie; their movie folders
// are still found and the scan names the loose files it left out.
func TestPlanAdoptGenreFolderWithLooseMovies(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	lib := lf.Path
	writeFile(t, filepath.Join(lib, "Horror", "Alien (1979).mkv"), "a")
	writeFile(t, filepath.Join(lib, "Horror", "Halloween.1978.1080p.BluRay.x264.mkv"), "h")
	writeFile(t, filepath.Join(lib, "Action", "Heat (1995)", "heat.avi"), "h")
	writeFile(t, filepath.Join(lib, "Action", "Speed (1994).mkv"), "s")
	writeFile(t, filepath.Join(lib, "Action", "Ronin (1998).mkv"), "r")
	// One movie with two cuts and a part stays one movie folder.
	writeFile(t, filepath.Join(lib, "Heat Cuts", "Heat (1995).mkv"), "1")
	writeFile(t, filepath.Join(lib, "Heat Cuts", "Heat (1995) - Directors Cut.mkv"), "2")
	// A movie folder with one loose video stays as it was.
	writeFile(t, filepath.Join(lib, "Dune - Part Two (2024)", "Dune.Part.Two.2024.mkv"), "d")
	writeFile(t, filepath.Join(lib, "No Year Movie", "movie.mkv"), "n")
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Action/Heat (1995)", "Dune - Part Two (2024)", "Heat Cuts", "No Year Movie"}
	if got := folderNames(p); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("folders %q\nwant %q", got, want)
	}
	if fmt.Sprint(p.LooseFolders) != "[Action Horror]" || p.LooseMovies != 4 || p.Warning != "" {
		t.Fatalf("loose %v %d warning %q", p.LooseFolders, p.LooseMovies, p.Warning)
	}
	if c := byFolder(p)["Action/Heat (1995)"]; c.Status != "matched" {
		t.Fatalf("heat %+v", c)
	}
}

// Loose movies right in the Library folder are offered as movies,
// with Tidy my folders as an option; a Library with none gets no note.
func TestPlanAdoptLooseMoviesAtTop(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Alien (1979).mkv"), "a")
	writeFile(t, filepath.Join(lf.Path, "Heat (1995).mkv"), "h")
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(folderNames(p)) != "[Alien (1979).mkv Heat (1995).mkv]" || len(p.LooseFolders) != 1 || p.LooseFolders[0] != "" || p.LooseMovies != 0 || p.LooseTop != 2 {
		t.Fatalf("folders %v loose %q %d top %d", folderNames(p), p.LooseFolders, p.LooseMovies, p.LooseTop)
	}
	lf2 := addFolder(t, s)
	writeFile(t, filepath.Join(lf2.Path, "Heat (1995)", "heat.mkv"), "h")
	if p, _ := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf2.ID); p.LooseMovies != 0 || p.LooseTop != 0 || p.SuggestFlat {
		t.Fatalf("note %+v", p)
	}
}
