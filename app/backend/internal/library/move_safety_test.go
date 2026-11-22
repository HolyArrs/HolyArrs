package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Two files of one size whose first and last MiB match are still two
// different files. The move must keep the source, since removing it
// would leave only the other file.
func TestMoveFileRefusesTargetThatSharesOnlyItsEnds(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	body := make([]byte, 3<<20)
	other := make([]byte, len(body))
	copy(other, body)
	other[len(other)/2] = 1 // differs only in the middle MiB
	writeFile(t, from, string(body))
	writeFile(t, to, string(other))
	err := moveFile(from, to)
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("err %v", err)
	}
	if b, _ := os.ReadFile(from); len(b) != len(body) || b[len(b)/2] != 0 {
		t.Fatal("source lost")
	}
}

// A target with the same bytes in full is the finished copy of a cut-off
// move, so the source goes.
func TestMoveFileFinishesWhenTargetMatchesInFull(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	body := strings.Repeat("x", 3<<20)
	writeFile(t, from, body)
	writeFile(t, to, body)
	if err := moveFile(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatal("source left behind")
	}
	if b, _ := os.ReadFile(to); string(b) != body {
		t.Fatal("target changed")
	}
}

// The real move waits for the file lock, as imports, renames and deletes
// do, so it never acts on a plan another file change made stale.
func TestMoveWaitsForTheFileLock(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	from := addFolder(t, s)
	to, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Second"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	svc, _, _ := runQueue(t, s, &metadata.Static{})
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	sl := slotsOf(t, s, id)[0]
	dir := filepath.Join(from.Path, "Dune - Part Two (2024)")
	writeFile(t, filepath.Join(dir, "dune.mkv"), "movie bytes")
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "files", &mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: sl.VersionID, RelativePath: "Dune - Part Two (2024)/dune.mkv", Size: 11, AddedAt: now,
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	unlock := s.LockFiles()
	if _, err := svc.Move(ctx, id, to.ID, true, domain.System); err != nil {
		unlock()
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	_, still := os.Stat(filepath.Join(dir, "dune.mkv"))
	unlock()
	if still != nil {
		t.Fatal("the move ran while another change held the file lock")
	}
	js := waitJobs(t, s, 1)
	if js[0].State != "done" {
		t.Fatalf("move %+v", js[0])
	}
	if b, err := os.ReadFile(filepath.Join(to.Path, "Dune - Part Two (2024)", "dune.mkv")); err != nil || string(b) != "movie bytes" {
		t.Fatalf("moved file %q %v", b, err)
	}
}
