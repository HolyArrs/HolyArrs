package library

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
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
	if _, changed := InspectLibraryVideo(root, "Movie (2020)/Movie.mkv", old, false); changed {
		t.Fatal("unchanged file read again without a backfill")
	}
	info, changed := InspectLibraryVideo(root, "Movie (2020)/Movie.mkv", old, true)
	if !changed || importer.NeedsFacts(info) {
		t.Fatalf("backfill did not read: %v %+v", changed, info)
	}
	if _, stale := info["resolution"]; stale || info["edition"] != "Director's Cut" {
		t.Fatalf("facts after backfill %+v", info)
	}
	// A read that fails on an unchanged file (a share timing out) keeps
	// the old facts and leaves the file for a later rescan.
	bad := filepath.Join(root, "Movie (2020)", "Bad.mkv")
	if err := os.WriteFile(bad, []byte{0x1a, 0x45, 0xdf, 0xa3, 0x00}, 0o666); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(bad)
	old["inspection_size"], old["inspection_modified"] = float64(fi.Size()), strconv.FormatInt(fi.ModTime().UnixNano(), 10)
	if info, changed := InspectLibraryVideo(root, "Movie (2020)/Bad.mkv", old, true); changed || info["resolution"] != "2160p" {
		t.Fatalf("failed backfill replaced facts: %v %+v", changed, info)
	}
	// A changed file is still read, and its failure stored.
	old["inspection_size"] = float64(1)
	if info, changed := InspectLibraryVideo(root, "Movie (2020)/Bad.mkv", old, false); !changed || info["inspection_status"] != "error" {
		t.Fatalf("changed file: %v %+v", changed, info)
	}
}
