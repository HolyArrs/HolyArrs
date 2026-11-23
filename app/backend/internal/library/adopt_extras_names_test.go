package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file named like an extra is still the movie when its name is the
// movie's own title, and loose extras in the Library folder are skipped.
func TestExtraNamedLikeTheTitle(t *testing.T) {
	for _, c := range []struct {
		folder, rel string
		want        bool
	}{
		{"Heat Interview (2001)", "Heat-Interview.mkv", false},
		{"Heat Interview (2001)", "Heat.Interview.2001.1080p.mkv", false},
		{"Heat (1995)", "Heat-Interview.mkv", true},
		{"Heat Interview (2001)", "Extras/Heat-Interview.mkv", true},
		{"Heat (1995)", "Heat (1995)-trailer.mkv", true},
		{"Big Short, The (2015)", "The.Big.Short.mkv", false},
		{"Big Short, The (2015)", "The.Big.Short-short.mkv", true},
		{"The Other Guys (2010)", "The.Other.Guys.mkv", false},
		{"Heat (1995)", "Deleted.Scenes.mkv", true},
		{"Heat (1995)", "Heat.1995.trailer.mkv", true},
		{"Heat (1995)", "Heat-short.mkv", true},
	} {
		if got := movieExtraIn(c.folder, c.rel); got != c.want {
			t.Errorf("movieExtraIn(%q, %q) = %v, want %v", c.folder, c.rel, got, c.want)
		}
	}
}

func TestPlanUsesTitleNamedFile(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Heat Interview (2001)", "Heat-Interview.mkv"), strings.Repeat("h", 50))
	plan, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m := byFolder(plan)["Heat Interview (2001)"].Main; m != "Heat Interview (2001)/Heat-Interview.mkv" {
		t.Fatalf("main file %q", m)
	}
}

func TestLooseRootExtrasSkipped(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"Heat (1995).mkv", "Heat (1995)-trailer.mkv", "Heat (1995) - Featurette.mkv", "The-Interview.mkv", "The.Big.Short.mkv", "The_Big_Short.mkv", "Deleted.Scenes.mkv"} {
		writeFile(t, filepath.Join(dir, n), "x")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := looseGroups(dir, ents)
	for k, vids := range got {
		for _, v := range vids {
			if MovieExtra(v.Path) {
				t.Errorf("group %q holds extra %q", k, v.Path)
			}
		}
	}
	for _, n := range []string{"The-Interview.mkv", "The.Big.Short.mkv", "The_Big_Short.mkv", "Deleted.Scenes.mkv"} {
		if _, ok := got[n]; !ok {
			t.Errorf("%s dropped: %v", n, got)
		}
	}
}

// A single collection file named like a title ending in an extras word is
// still offered as the movie.
func TestSingleCandidateTitleNotExtra(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Collection", "The.Big.Short.mkv"), "x")
	if _, ok := mainCandidateVideo(root, "Collection/The.Big.Short.mkv", nil, false); !ok {
		t.Fatal("The.Big.Short.mkv rejected as an extra")
	}
	writeFile(t, filepath.Join(root, "Collection", "Heat (1995)-trailer.mkv"), "x")
	if _, ok := mainCandidateVideo(root, "Collection/Heat (1995)-trailer.mkv", nil, false); ok {
		t.Fatal("a trailer offered as the movie")
	}
}
