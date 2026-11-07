package downloader

import (
	"context"
	"database/sql"
	"testing"
)

func removalPending(t *testing.T, w *world, id int64) sql.NullBool {
	t.Helper()
	var p sql.NullBool
	if err := w.s.Read().QueryRow("SELECT removal_pending FROM downloads WHERE id = ?", id).Scan(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

// 4.1.3 minor 7: when the Downloader cannot be reached at Remove, the
// removal is recorded and done on a later sync, with the same file choice.
func TestRemoveRetriesWhenTheDownloaderWasDown(t *testing.T) {
	w, m := setup(t)
	letChange(t, m) // it removes an item already in the Downloader
	id := grabbed(t, w, m)
	w.fake.mu.Lock()
	w.fake.refuse = true
	w.fake.mu.Unlock()
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if p := removalPending(t, w, id); !p.Valid || !p.Bool {
		t.Fatalf("pending %+v", p)
	}
	_, _ = m.SyncOnce(context.Background())
	if p := removalPending(t, w, id); !p.Valid {
		t.Fatal("pending cleared while the Downloader is still down")
	}
	w.fake.mu.Lock()
	w.fake.refuse = false
	w.fake.mu.Unlock()
	_, _ = m.SyncOnce(context.Background())
	if len(w.fake.deleted) != 1 || w.fake.deleted[0] != hash+"|true" {
		t.Fatalf("deleted %v", w.fake.deleted)
	}
	if p := removalPending(t, w, id); p.Valid {
		t.Fatalf("still pending %+v", p)
	}
	_, _ = m.SyncOnce(context.Background())
	if len(w.fake.deleted) != 1 {
		t.Fatalf("removed twice %v", w.fake.deleted)
	}
}
