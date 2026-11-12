package downloads

import (
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A path pair with the same folder on both sides is kept as typed: its
// HolyArrs side is also how HolyArrs knows the Downloader's folder when no
// save folder covers it (by-hand imports, clearing finished downloads,
// deleting files rTorrent and aria2 cannot).
func TestSamePathPairIsKept(t *testing.T) {
	dir := t.TempDir()
	job := filepath.Join(dir, "Some.Show.S01")
	if err := os.Mkdir(job, 0o750); err != nil {
		t.Fatal(err)
	}
	in := []domain.PathMap{{Theirs: dir, Ours: dir}, {Theirs: "/downloads", Ours: "/data/downloads"}}
	var d domain.Downloader
	if err := fill(Input{Name: "r", Adapter: "rtorrent", URL: "http://r:8080", PathMap: in}, &d, true); err != nil {
		t.Fatal(err)
	}
	if len(d.PathMap) != 2 || d.PathMap[0] != in[0] {
		t.Fatalf("stored %+v", d.PathMap)
	}
	got, why := safeDeleteTarget(deleteAsk{client: "rTorrent", content: job, name: "Some.Show.S01", pathMap: d.PathMap}, nil)
	if why != "" || got.root != filepath.Clean(dir) || got.rel != "Some.Show.S01" {
		t.Fatalf("with only the same-path pair: %+v %q", got, why)
	}
}
