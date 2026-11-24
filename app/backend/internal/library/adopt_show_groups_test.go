package library

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestShowFoldersReadsGroupFolders(t *testing.T) {
	root := t.TempDir()
	mk := func(p string) {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(p)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := func(p string) {
		mk(filepath.Dir(p))
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("O/The Office (2005)/Season 01")
	file("O/Oz/Oz S01E01.mkv")          // a letter folder: show folders with episodes loose
	mk("Anime/Frieren (2023)/Season 1") // a genre folder: its folders hold seasons
	mk("Dexter/Season 1")               // a show with seasons, not a group
	file("V/V S01E01.mkv")              // a show named V, not a letter folder
	mk("Lost/Specials")                 // Specials marks a show folder
	mk("Westworld/Westworld S01 1080p") // a show with oddly named season folders
	mk(".hidden/Show")
	ents, _ := os.ReadDir(root)
	got := ShowFolders(root, ents)
	want := []string{"Anime/Frieren (2023)", "Dexter", "Lost", "O/Oz", "O/The Office (2005)", "V", "Westworld"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if after := AfterIn(got, "Lost"); !reflect.DeepEqual(after, []string{"O/Oz", "O/The Office (2005)", "V", "Westworld"}) {
		t.Fatalf("after: %q", after)
	}
}
