package library

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/importer"
)

// An unchanged file with facts from before the full header reader is read
// again only when the rescan allows a backfill; a fresh read drops stale
// header facts and keeps a confirmed edition.
func TestInspectLibraryVideoBackfill(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "Movie (2020)", "Movie.mkv")
	if err := os.MkdirAll(filepath.Dir(full), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("not a real video"), 0o666); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(full)
	old := map[string]any{"inspection_status": "complete", "inspection_size": float64(fi.Size()),
		"inspection_modified": strconv.FormatInt(fi.ModTime().UnixNano(), 10), "edition": "Director's Cut", "resolution": "2160p"}
	if _, changed := inspectLibraryVideo(root, "Movie (2020)/Movie.mkv", old, false); changed {
		t.Fatal("unchanged file read again without a backfill")
	}
	info, changed := inspectLibraryVideo(root, "Movie (2020)/Movie.mkv", old, true)
	if !changed || importer.NeedsFacts(info) {
		t.Fatalf("backfill did not read: %v %+v", changed, info)
	}
	if _, stale := info["resolution"]; stale || info["edition"] != "Director's Cut" {
		t.Fatalf("facts after backfill %+v", info)
	}
}
