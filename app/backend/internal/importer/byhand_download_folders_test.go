package importer

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func refusedPlace(t *testing.T, err error) bool {
	t.Helper()
	var pe *PlaceError
	return err != nil && (errors.As(err, &pe) || errors.Is(err, ErrByHandPath))
}

// Import files by hand on a finished download HolyArrs sent opens that one
// download's own folder, which may be read even with no path pair and no
// save folder set in HolyArrs; the rest of the Downloader's save folder may
// not.
func TestByHandPlacesHoldOwnDownloadOnly(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	im := &Importer{Store: w.s}
	content := filepath.Join(w.dl, packName)
	if got := im.ReviewFolder(ctx, w.dlID); got != content {
		t.Fatalf("review folder %q, want %q", got, content)
	}
	if sc, err := im.ScanByHand(ctx, content); err != nil || len(sc.Files) != 2 {
		t.Fatalf("scan of the download's folder %+v %v", sc, err)
	}
	sibling := filepath.Join(w.dl, "Other.Show.S01")
	write(t, filepath.Join(sibling, "a.mkv"), 10)
	for _, p := range []string{w.dl, sibling, filepath.Join(sibling, "a.mkv"), content + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(sibling)} {
		if _, err := im.ScanByHand(ctx, p); !refusedPlace(t, err) {
			t.Errorf("scan %s: %v", p, err)
		}
	}

	// One loose file: only that file.
	loose := filepath.Join(w.dl, "Copper.Valley.S01E03.mkv")
	write(t, loose, 10)
	exec := func(q string, args ...any) {
		t.Helper()
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE downloads SET files_seen = '[{"name":"Copper.Valley.S01E03.mkv","size":10}]' WHERE id = ?`, w.dlID)
	if got := im.ReviewFolder(ctx, w.dlID); got != loose {
		t.Fatalf("review of a loose file %q", got)
	}
	ps, _ := im.ByHandPlaces(ctx)
	if !ps.Holds(loose) || ps.Holds(w.dl) || ps.Holds(content) {
		t.Fatalf("loose file places %+v", ps)
	}

	// Not sent by HolyArrs (no Release): nothing of it counts.
	exec(`UPDATE downloads SET release_id = NULL, files_seen = json_array(json_object('name', ?, 'size', 10)) WHERE id = ?`, packName+"/"+ep(1), w.dlID)
	if ps, _ = im.ByHandPlaces(ctx); ps.Holds(content) || ps.Holds(w.dl) {
		t.Fatalf("a download HolyArrs did not send counts: %+v", ps)
	}
	if im.ReviewFolder(ctx, w.dlID) != "" {
		t.Fatal("review folder for a download HolyArrs did not send")
	}
	exec("UPDATE downloads SET release_id = ? WHERE id = ?", w.relID, w.dlID)

	// A disk root or the data folder reported as the save folder adds nothing.
	root := filepath.VolumeName(w.dl) + string(filepath.Separator)
	exec(`UPDATE downloads SET save_path = ?, files_seen = '[{"name":"a.mkv","size":10}]' WHERE id = ?`, root, w.dlID)
	if ps, _ = im.ByHandPlaces(ctx); len(ps.Downloads) != 0 {
		t.Fatalf("a disk root's file counts: %v", ps.Downloads)
	}
	exec(`UPDATE downloads SET save_path = ?, files_seen = json_array(json_object('name', ?, 'size', 10)) WHERE id = ?`,
		filepath.Dir(w.s.Dir()), filepath.Base(w.s.Dir())+"/a.mkv", w.dlID)
	if ps, _ = im.ByHandPlaces(ctx); ps.Holds(w.s.Dir()) || len(ps.Downloads) != 0 {
		t.Fatalf("the data folder counts: %v", ps.Downloads)
	}

	// A removed Downloader's downloads count no more.
	exec("UPDATE downloads SET save_path = ?, files_seen = json_array(json_object('name', ?, 'size', 10)) WHERE id = ?", w.dl, packName+"/"+ep(1), w.dlID)
	if ps, _ = im.ByHandPlaces(ctx); !ps.Holds(content) {
		t.Fatalf("own download refused again: %+v", ps)
	}
	exec("UPDATE downloaders SET removed = 1")
	if ps, _ = im.ByHandPlaces(ctx); ps.Holds(content) {
		t.Error("a removed Downloader's download is still a place")
	}
}
