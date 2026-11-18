package downloads

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// cardReading reports what the card of the download says about a folder read.
func cardReading(t *testing.T, a *Web, id int64) bool {
	t.Helper()
	out, err := a.decisions(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, dv := range out {
		if dv.DownloadID != nil && *dv.DownloadID == id {
			return dv.Reading
		}
	}
	t.Fatal("no card for the download")
	return false
}

// While Import again reads the folder in the background, the card says so; when the read ends it does not.
func TestImportAgainCardSaysReading(t *testing.T) {
	w, m, _, id := slowFolder(t, 20*time.Millisecond, 300*time.Millisecond)
	a := &Web{Store: w.s, Downloads: m}
	if cardReading(t, a, id) {
		t.Fatal("reading before the click")
	}
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	if !cardReading(t, a, id) {
		t.Fatal("the card does not say the folder is read")
	}
	readDone(t, m)
	if cardReading(t, a, id) {
		t.Fatal("still reading after the read ended")
	}
}

// A download that does not wait answers at once, before any read and also while one runs.
func TestImportAgainChecksBeforeTheRead(t *testing.T) {
	w, m, e, id := slowFolder(t, 20*time.Millisecond, time.Minute)
	if err := m.ImportAgain(context.Background(), 999999, luis); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("missing download: %v", err)
	}
	w.exec(t, "UPDATE slots SET state = 'wanted', last_reason = NULL WHERE id = ?", w.slotID)
	if err := m.ImportAgain(context.Background(), id, luis); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("no waiting slot: %v", err)
	}
	if n := e.asked.Load(); n != 0 {
		t.Fatalf("the Downloader was asked %d times", n)
	}
	w.exec(t, "UPDATE slots SET state = 'downloading' WHERE id = ?", w.slotID)
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	w.exec(t, "UPDATE downloads SET state = 'downloading' WHERE id = ?", id)
	if err := m.ImportAgain(context.Background(), id, luis); !errors.Is(err, ErrNotWaiting) {
		t.Fatalf("second click on a download that does not wait: %v", err)
	}
	close(e.release)
	readDone(t, m)
}
