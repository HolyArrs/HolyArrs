package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// replaceDownloader removes the first Downloader the way Settings does and,
// when again is set, adds a new one at the same address.
func replaceDownloader(t *testing.T, w *world, again bool) int64 {
	t.Helper()
	old := w.downloader(t, 1)
	w.exec(t, "UPDATE downloaders SET removed = 1, state = 'disabled', name = name || ' (removed 1)' WHERE id = 1")
	if !again {
		return 0
	}
	return w.addDownloader(t, &domain.Downloader{Name: "qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: old.URL, Priority: 1, AllowLocal: true})
}

func orphanRows(t *testing.T, w *world, m *Manager) []DownloadRow {
	t.Helper()
	a := &Web{Store: w.s, Downloads: m}
	rec := httptest.NewRecorder()
	a.DownloadsView(rec, httptest.NewRequest("GET", "/api/v1/downloads/view", nil))
	if rec.Code != 200 {
		t.Fatalf("view %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Downloads []DownloadRow `json:"downloads"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Downloads
}

// A download whose Downloader was removed and added again follows the new one
// when the new one holds the same torrent, and imports when it finishes.
func TestReplacedDownloaderRelinksByHash(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	nid := replaceDownloader(t, w, true)
	w.fake.set(hash, map[string]any{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "category": "holyarrs", "progress": 1.0, "state": "uploading", "save_path": "/data/downloads/complete"})
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := w.download(t, id)
	if d.DownloaderID != nid {
		t.Fatalf("still on Downloader %d, want %d", d.DownloaderID, nid)
	}
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State != "finished" || len(w.finshed) != 1 || w.finshed[0] != id {
		t.Fatalf("download %s, import asked for %v", d.State, w.finshed)
	}
}

// A finished download left on a removed Downloader is moved over too, and its
// import is asked for again.
func TestReplacedDownloaderRelinksFinished(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
	w.exec(t, "UPDATE slots SET state = 'stalled' WHERE id = ?", w.slotID)
	nid := replaceDownloader(t, w, true)
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.DownloaderID != nid {
		t.Fatalf("still on Downloader %d, want %d", d.DownloaderID, nid)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotDownloading {
		t.Fatalf("slot %s, want downloading so the import takes it", sl.State)
	}
	if len(w.finshed) != 1 || w.finshed[0] != id {
		t.Fatalf("import asked for %v", w.finshed)
	}
}

// An imported download is known by its hash too: once the same Downloader is
// added again it moves over (no import asked for), and the Downloads page
// shows it once, as HolyArrs' own, not a second time as an outside torrent.
func TestReplacedDownloaderRelinksImported(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
	w.exec(t, "UPDATE slots SET state = 'in_library', active_download_id = NULL WHERE id = ?", w.slotID)
	nid := replaceDownloader(t, w, true)
	w.fake.set(hash, map[string]any{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "category": "holyarrs", "progress": 1.0, "state": "stoppedUP", "save_path": "/data/downloads/complete"})
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.DownloaderID != nid || d.State != "finished" {
		t.Fatalf("download on Downloader %d (%s), want %d", d.DownloaderID, d.State, nid)
	}
	if len(w.finshed) != 0 {
		t.Fatalf("import asked for again: %v", w.finshed)
	}
	a := &Web{Store: w.s, Downloads: m}
	feedStore(t, a)
	rec := httptest.NewRecorder()
	a.DownloadsView(rec, httptest.NewRequest("GET", "/api/v1/downloads/view", nil))
	var out struct {
		Downloads []DownloadRow `json:"downloads"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	var mine []DownloadRow
	for _, r := range out.Downloads {
		if r.Hash == hash {
			mine = append(mine, r)
		}
	}
	if len(mine) != 1 || mine[0].ID != id || mine[0].Outside || mine[0].DownloaderGone {
		t.Fatalf("rows for the torrent: %+v", mine)
	}
}

// A new Downloader that does not hold the torrent takes nothing over, and a
// download already there under the same hash is left alone.
func TestReplacedDownloaderKeepsRowWithoutHash(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	replaceDownloader(t, w, false)
	_, other := startFake(t)
	w.addDownloader(t, &domain.Downloader{Name: "Other", Adapter: "qbittorrent", Protocol: "torrent", URL: other, Priority: 2, AllowLocal: true})
	m.relinkOrphans(context.Background())
	if d := w.download(t, id); d.DownloaderID != 1 {
		t.Fatalf("moved to Downloader %d", d.DownloaderID)
	}
}

// With no Downloader holding it, the row still has Remove, which clears
// HolyArrs' own record and frees the Slot without calling any Downloader.
func TestOrphanedFinishedDownloadCanBeRemoved(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
	w.exec(t, "UPDATE slots SET state = 'stalled' WHERE id = ?", w.slotID)
	replaceDownloader(t, w, false)
	rows := orphanRows(t, w, m)
	if len(rows) != 1 || !rows[0].CanAct || !rows[0].ClientLocked || !rows[0].DownloaderGone {
		t.Fatalf("row %+v", rows)
	}
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	d := w.download(t, id)
	if d.State != "cleaned" {
		t.Fatalf("download %s", d.State)
	}
	if len(w.fake.deleted) != 0 {
		t.Fatalf("asked a removed Downloader to delete: %v", w.fake.deleted)
	}
	var pending any
	if err := w.s.Read().QueryRow("SELECT removal_pending FROM downloads WHERE id = ?", id).Scan(&pending); err != nil || pending != nil {
		t.Fatalf("removal pending %v (%v)", pending, err)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotWanted {
		t.Fatalf("slot %s", sl.State)
	}
}

// A finished download whose import never ran offers Import again, which puts
// a stalled Slot back to downloading and queues the import.
func TestImportAgainForStuckFinishedDownload(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	if err := m.ImportAgain(context.Background(), id, luis); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("import again while downloading: %v", err)
	}
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
	w.exec(t, "UPDATE slots SET state = 'stalled' WHERE id = ?", w.slotID)
	rows := orphanRows(t, w, m)
	if len(rows) != 1 || !rows[0].CanImport || rows[0].DownloaderGone {
		t.Fatalf("row %+v", rows)
	}
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotDownloading {
		t.Fatalf("slot %s", sl.State)
	}
	if len(w.finshed) != 1 || w.finshed[0] != id {
		t.Fatalf("import asked for %v", w.finshed)
	}
	w.exec(t, "UPDATE slots SET state = 'in_library' WHERE id = ?", w.slotID)
	if err := m.ImportAgain(context.Background(), id, luis); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("import again after the import: %v", err)
	}
}
