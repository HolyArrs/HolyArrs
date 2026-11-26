package shows

import (
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// Header facts are read before the write; inside it adopting only
// uses what was read and never opens a file, so a slow disk can't hold the
// write past its time cap.
func TestAdoptFileInfoReadsOnlyWhenAsked(t *testing.T) {
	root := t.TempDir()
	af := library.AdoptEpisodeFile{Path: "Show/Season 1/Show.S01E01.1080p.WEB.mkv", Keys: [][2]int{{1, 1}}}
	writeFile(t, filepath.Join(root, filepath.FromSlash(af.Path)), "x")
	got := inspectAdoptFiles(root, []library.AdoptEpisodeFile{af, {Path: "Show/extra.mkv"}})
	if len(got) != 1 || got[af.Path]["inspection_size"] == nil {
		t.Fatalf("inspected %v", got)
	}
	if info := adoptFileInfo(root, af, false); len(info) != 1 || info["step"] == nil {
		t.Fatalf("no-read info %v", info)
	}
}
