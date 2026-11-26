package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestWatchPass(t *testing.T) {
	w := showSetup(t, []int{2})
	ctx := context.Background()
	drop := t.TempDir()
	old := time.Now().Add(-time.Hour)
	for _, n := range []int{1, 2, 3} {
		p := filepath.Join(drop, "pack", ep(n))
		write(t, p, 2048)
		if n != 3 {
			_ = os.Chtimes(p, old, old)
		}
	}
	sample := filepath.Join(drop, "pack", "Sample", "sample-"+ep(4))
	write(t, sample, 10)
	_ = os.Chtimes(sample, old, old)
	im := &Importer{Store: w.s}
	if err := im.WatchPass(ctx); err != nil { // nothing watched yet
		t.Fatal(err)
	}
	if _, err := SaveWatchFolders(ctx, w.s, []string{drop}, domain.System); err != nil {
		t.Fatal(err)
	}
	if ps, err := im.ByHandPlaces(ctx); err != nil || len(ps.Watch) != 1 {
		t.Fatalf("places %+v %v", ps, err)
	}
	if err := im.WatchPass(ctx); err != nil {
		t.Fatal(err)
	}
	gone := func(n int) bool { _, err := os.Stat(filepath.Join(drop, "pack", ep(n))); return os.IsNotExist(err) }
	if !gone(1) {
		t.Fatal("ep1 matched an empty Slot and should have moved in")
	}
	if gone(2) || gone(3) {
		t.Fatal("a filled Slot and a file still being copied stay")
	}
	if _, err := os.Stat(sample); err != nil {
		t.Fatal("samples stay")
	}
	// Once settled, the copied-in file goes on a later pass.
	_ = os.Chtimes(filepath.Join(drop, "pack", ep(3)), old, old)
	if err := im.WatchPass(ctx); err != nil {
		t.Fatal(err)
	}
	if !gone(3) || gone(2) {
		t.Fatal("ep3 moves in after it settles; ep2 still stays")
	}
}

func TestSaveWatchFoldersRefusesBad(t *testing.T) {
	w := showSetup(t, nil)
	if _, err := SaveWatchFolders(context.Background(), w.s, []string{"relative"}, domain.System); err == nil {
		t.Fatal("relative path saved")
	}
}

// A watched folder that is also a Downloader's folder may hold files still
// seeding: they are copied in and left where they are.
func TestWatchPassCopiesFromDownloaderFolder(t *testing.T) {
	w := showSetup(t, nil)
	ctx := context.Background()
	drop := t.TempDir()
	p := filepath.Join(drop, "pack", ep(1))
	write(t, p, 2048)
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(p, old, old)
	im := &Importer{Store: w.s, SavePath: drop}
	if _, err := SaveWatchFolders(ctx, w.s, []string{drop}, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := im.WatchPass(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("a file in a Downloader's folder was moved away")
	}
	var n int
	if err := w.s.Read().QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&n); err != nil || n == 0 {
		t.Fatalf("not imported: %d %v", n, err)
	}
}
