package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// countReads makes every content read of the library code count.
func countReads(t *testing.T) *int {
	t.Helper()
	n := 0
	openToRead = func(name string) (*os.File, error) {
		n++
		return os.Open(name)
	}
	t.Cleanup(func() { openToRead = os.Open })
	return &n
}

func namingWithQuality(t *testing.T, ctx context.Context, svc *Service) {
	t.Helper()
	if err := mediafiles.PutNamingSettings(ctx, svc.Store, mediafiles.NamingSettings{FilePattern: "{title} ({year}) [{quality}] {group}", FolderPattern: mediafiles.DefaultFolderPattern,
		Colon: mediafiles.ColonSpaceDashSpace, Illegal: mediafiles.IllegalReplace, RenameOnImport: true}, domain.System); err != nil {
		t.Fatal(err)
	}
}

// A rename inside one Library folder is a plain rename: the video is never
// read, however large it is.
func TestRenameNeverReadsTheFile(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, id, "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv", "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS", "imported")
	namingWithQuality(t, ctx, svc)
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	reads := countReads(t)
	var seen [][2]int
	unlock := s.LockFiles()
	n, err := svc.applyRenameLocked(ctx, plan, 0, domain.System, func(done, total int) { seen = append(seen, [2]int{done, total}) })
	unlock()
	if err != nil || n != 1 {
		t.Fatalf("renamed %d: %v", n, err)
	}
	if *reads != 0 {
		t.Fatalf("the rename read file content %d times; a plain rename reads nothing", *reads)
	}
	if got := fileRow(t, s, f.ID).RelativePath; got == f.RelativePath {
		t.Fatal("not renamed")
	}
	if len(seen) != 1 || seen[0] != [2]int{1, 1} {
		t.Fatalf("progress %v, want one step of 1 of 1", seen)
	}
}

// A move cut off after a copy to another disk, with the source already gone,
// is still recognised as ours on the next run; a different file of the same
// size at the new name is not.
func TestRecordedCopyResumesWithoutReading(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b", "a.mkv")
	writeFile(t, from, "video")
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(from, old, old); err != nil {
		t.Fatal(err)
	}
	crossDevice(t)
	proof, err := fileProof(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := moveRecordedFile(from, to, proof); err != nil {
		t.Fatal(err)
	}
	reads := countReads(t)
	if err := moveRecordedFile(from, to, proof); err != nil {
		t.Fatalf("the copied file was not recognised: %v", err)
	}
	if *reads != 0 {
		t.Fatalf("recognising the copy read file content %d times", *reads)
	}
	writeFile(t, to, "other")
	if err := moveRecordedFile(from, to, proof); err == nil {
		t.Fatal("a different file of the same size was taken as the recorded one")
	}
}

// Steps written by an earlier version hold the size and a SHA-256 of the
// content; they are still recognised.
func TestOldContentProofStillRecognised(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv")
	writeFile(t, to, "video")
	sum := sha256.Sum256([]byte("video"))
	proof := strconv.Itoa(len("video")) + ":" + hex.EncodeToString(sum[:])
	if err := moveRecordedFile(from, to, proof); err != nil {
		t.Fatalf("old proof not recognised: %v", err)
	}
	writeFile(t, to, "vidEo")
	if err := moveRecordedFile(from, to, proof); err == nil {
		t.Fatal("old proof matched different content")
	}
}
