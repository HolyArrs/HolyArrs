package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Import files by hand lists a link to a file in a download's folder as that
// file once links are followed, with its target's size; links to folders and
// links not followed stay out.
func TestByHandScanListsFollowedLinks(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	im := &Importer{Store: w.s}
	content := filepath.Join(w.dl, packName)
	mount := t.TempDir()
	target := filepath.Join(mount, "real.mkv")
	write(t, target, 42)
	link := filepath.Join(content, "Copper.Valley.S01E03.mkv")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if err := os.Symlink(mount, filepath.Join(content, "linked-folder")); err != nil {
		t.Fatal(err)
	}
	old := FollowLinks
	defer func() { FollowLinks = old }()

	FollowLinks = false
	if sc, err := im.ScanByHand(ctx, content); err != nil || len(sc.Files) != 2 {
		t.Fatalf("not followed: %+v %v", sc, err)
	}
	FollowLinks = true
	sc, err := im.ScanByHand(ctx, content)
	if err != nil || len(sc.Files) != 3 {
		t.Fatalf("followed: %+v %v", sc, err)
	}
	for _, f := range sc.Files {
		if f.Path == link && f.Size != 42 {
			t.Fatalf("the link counts as its target: %+v", f)
		}
	}
}
