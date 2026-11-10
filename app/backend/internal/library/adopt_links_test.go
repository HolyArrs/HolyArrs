package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// A movie file that is a link to a mounted file is found only when
// links are followed.
func TestListVideosFollowsFileLinks(t *testing.T) {
	mount, lib := t.TempDir(), t.TempDir()
	target := filepath.Join(mount, "real.mkv")
	if err := os.WriteFile(target, make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lib, "Movie (2020)"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(lib, "Movie (2020)", "Movie (2020).mkv")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	old := importer.FollowLinks
	defer func() { importer.FollowLinks = old }()

	importer.FollowLinks = false
	if got := ListVideosN(lib, "Movie (2020)", 10); len(got) != 0 {
		t.Fatalf("strict default listed a link: %+v", got)
	}
	importer.FollowLinks = true
	got := ListVideosN(lib, "Movie (2020)", 10)
	if len(got) != 1 || got[0].Size != 2048 {
		t.Fatalf("want the linked file with its target size, got %+v", got)
	}
}
