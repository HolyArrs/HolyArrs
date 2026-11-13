package importer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A Downloader's own save folder, as it reports it on a finished download,
// is a place to import by hand from, even with no path pair and no save
// folder set in HolyArrs: Import files by hand on a refused download opens it.
func TestByHandPlacesHoldDownloaderSaveFolder(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	im := &Importer{Store: w.s}
	ps, err := im.ByHandPlaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ps.Holds(w.dl) || !ps.Holds(filepath.Join(w.dl, relName+".mkv")) {
		t.Fatalf("the Downloader's save folder %s is refused; places %+v", w.dl, ps)
	}
	if sc, err := im.ScanByHand(ctx, w.dl); err != nil || len(sc.Files) != 1 {
		t.Fatalf("scan of the save folder %+v %v", sc, err)
	}
	if im.ReviewFolder(ctx, w.dlID) == "" {
		t.Fatal("no review folder")
	}
	if _, err := im.ScanByHand(ctx, im.ReviewFolder(ctx, w.dlID)); err != nil {
		t.Fatalf("Import files by hand from the review folder: %v", err)
	}

	sibling := w.dl + "2"
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	var pe *PlaceError
	for _, p := range []string{sibling, filepath.Dir(w.dl), w.dl + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(sibling)} {
		if _, err := im.ScanByHand(ctx, p); err == nil || (!errors.As(err, &pe) && !errors.Is(err, ErrByHandPath)) {
			t.Errorf("scan %s: %v", p, err)
		}
	}

	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET save_path = ?", w.s.Dir())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if ps, _ = im.ByHandPlaces(ctx); ps.Holds(w.s.Dir()) {
		t.Error("a save folder over the data folder counts as a place")
	}

	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET save_path = ?", w.dl)
		if err == nil {
			_, err = tx.Exec("UPDATE downloaders SET removed = 1")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if ps, _ = im.ByHandPlaces(ctx); ps.Holds(w.dl) {
		t.Error("a removed Downloader's save folder is still a place")
	}
}
