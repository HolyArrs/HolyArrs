package migrate

import (
	"path/filepath"
	"testing"
)

func TestMarkFound(t *testing.T) {
	movies := &FolderPlan{OldPath: "/movies", Path: "/data/Movies", Kind: "movie"}
	left := &FolderPlan{OldPath: "/tv", Path: "/tv", Kind: "show", Left: true}
	p := &Plan{Groups: []Group{
		{Name: "folders", Items: []Item{{Folder: movies}, {Folder: left}}},
		{Name: "titles", Items: []Item{
			{Title: &TitlePlan{Library: "/data/Movies", Folder: "Dune (2021)"}},
			{Title: &TitlePlan{Library: "/data/Movies", Folder: "Heat (1995)"}},
			{Title: &TitlePlan{Library: "/data/Movies", Folder: "Gone", Unresolved: "no folder"}},
			{Title: &TitlePlan{Library: "/tv", Folder: "Lost"}},
		}},
	}}
	have := map[string]bool{filepath.FromSlash("/data/Movies"): true, filepath.FromSlash("/data/Movies/Dune (2021)"): true}
	markFound(p, func(s string) bool { return have[s] })
	if movies.Checked != 2 || movies.Found != 1 {
		t.Fatalf("movies: found %d of %d", movies.Found, movies.Checked)
	}
	if movies.Missing || left.Checked != 0 {
		t.Fatalf("left folder checked: %d", left.Checked)
	}
}

func TestMarkSuggest(t *testing.T) {
	movies := &FolderPlan{OldPath: "/data/Movies", Path: "/data/Movies", Kind: "movie", Unseen: true}
	p := &Plan{Groups: []Group{
		{Name: "folders", Items: []Item{{Folder: movies}}},
		{Name: "titles", Items: []Item{
			{Title: &TitlePlan{Library: "/data/Movies", Folder: "Dune (2021)"}},
			{Title: &TitlePlan{Library: "/data/Movies", Folder: "Heat (1995)"}},
		}},
	}}
	have := map[string]bool{filepath.FromSlash("/media/films/Dune (2021)"): true, filepath.FromSlash("/media/films/Heat (1995)"): true, filepath.FromSlash("/media/other/Dune (2021)"): true}
	markSuggest(p, []string{"/media/other", "/media/films", "/media/empty"}, func(s string) bool { return have[s] })
	if movies.Suggest != "/media/films" {
		t.Fatalf("suggest = %q", movies.Suggest)
	}
}

func TestMarkFoundSkipsTitlesWithoutFiles(t *testing.T) {
	movies := &FolderPlan{OldPath: "/movies", Path: "/data/Movies", Kind: "movie"}
	p := &Plan{Groups: []Group{
		{Name: "folders", Items: []Item{{Folder: movies}}},
		{Name: "titles", Items: []Item{
			{Title: &TitlePlan{Library: "/data/Movies", Folder: "Dune (2021)", hasFiles: true}},
			{Title: &TitlePlan{Library: "/data/Movies", Folder: "Waiting (2026)"}},
		}},
	}}
	have := map[string]bool{filepath.FromSlash("/data/Movies"): true, filepath.FromSlash("/data/Movies/Dune (2021)"): true}
	markFound(p, func(s string) bool { return have[s] })
	if movies.Checked != 1 || movies.Found != 1 {
		t.Fatalf("found %d of %d", movies.Found, movies.Checked)
	}
}

func TestDeepFoldersSkipsLibraries(t *testing.T) {
	kids := map[string][]string{"/m": {"/m/Movies", "/m/TV"}, "/m/TV": {"/m/TV/A"}}
	for i := 0; i < 60; i++ {
		kids["/m/Movies"] = append(kids["/m/Movies"], "/m/Movies/x"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	opened := map[string]bool{}
	got := deepFolders([]string{"/m"}, func(p string) []string { opened[p] = true; return kids[p] })
	if opened["/m/Movies/xaa"] || len(got) != 4 {
		t.Fatalf("opened a title folder or wrong list: %v", got)
	}
}
