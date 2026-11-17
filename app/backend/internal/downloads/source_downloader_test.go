package downloads

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestGrabGoesToTheSourcesOwnDownloader(t *testing.T) {
	w, m := setup(t)
	lab := w.downloader(t, 1)
	private := w.addDownloader(t, &domain.Downloader{Name: "Private qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: lab.URL,
		Priority: 40, AllowLocal: true, Tags: []string{"kids"}})
	sendSourceTo(t, w, private)
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.DownloaderID != private {
		t.Fatalf("sent to Downloader %d, want the Source's own %d", d.DownloaderID, private)
	}
}

func TestGrabNeverFallsBackWhenTheSourcesDownloaderIsDown(t *testing.T) {
	w, m := setup(t)
	lab := w.downloader(t, 1)
	private := w.addDownloader(t, &domain.Downloader{Name: "Private qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: lab.URL,
		Priority: 40, AllowLocal: true})
	sendSourceTo(t, w, private)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET backoff_until = ? WHERE id = ?", database.FormatTime(time.Now().Add(time.Hour)), private)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil {
		t.Fatal("Grab sent it somewhere while the Source's Downloader rests")
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM downloads").Scan(&n)
	if n != 0 || len(w.fake.added) != 0 {
		t.Fatalf("it fell back to another Downloader: %d downloads", n)
	}
}

func TestGrabUsesAnyDownloaderWhenTheSourcesOneWasRemoved(t *testing.T) {
	w, m := setup(t)
	lab := w.downloader(t, 1)
	gone := w.addDownloader(t, &domain.Downloader{Name: "Old qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: lab.URL,
		Priority: 40, AllowLocal: true})
	sendSourceTo(t, w, gone)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET removed = 1 WHERE id = ?", gone)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.DownloaderID != lab.ID {
		t.Fatalf("sent to Downloader %d, want %d", d.DownloaderID, lab.ID)
	}
}

func sendSourceTo(t *testing.T, w *world, downloader int64) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE sources SET downloader_id = ?", downloader)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
