package downloads

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

func lastData(t *testing.T, w *world, kind string, id int64) map[string]any {
	t.Helper()
	var raw string
	if err := w.s.ReadEvents().QueryRow("SELECT data FROM events WHERE kind = ? AND download_id = ? ORDER BY id DESC LIMIT 1", kind, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	d := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// History keeps whether the files went and, when they stayed, why, so the
// row can say it in the reader's language.
func TestRemoveDoneEventSaysFilesKeptOrDeleted(t *testing.T) {
	w, m, id, _, _, _ := doneOnDisk(t, "inplace")
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	d := lastData(t, w, "download_removed", id)
	if d["delete_files"] != false || !strings.Contains(fmt.Sprint(d["kept"]), "Library folder") {
		t.Fatalf("kept: %v", d)
	}
	w, m, id, _ = seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if d := lastData(t, w, "download_removed", id); d["delete_files"] != true || d["kept"] != nil {
		t.Fatalf("deleted: %v", d)
	}
}

// A Downloader HolyArrs holds back from for a moment is named plainly, never
// with the internal words, and the retry that works later is in History.
func TestRemoveDoneHeldBackSaidPlainlyAndRetryLogged(t *testing.T) {
	w, m, id, se := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	eng := &diskEngine{seedEngine: se, fail: fmt.Errorf("qbittorrent: %w", outbound.ErrSuppressed)}
	m.Engine = func(domain.Downloader) (Engine, error) { return eng, nil }
	note, err := m.RemoveSaid(context.Background(), id, RemoveChoice{}, luis, "luis")
	if err != nil || strings.Contains(note, "outside service") || !strings.Contains(note, "is not answering right now") {
		t.Fatalf("%q %v", note, err)
	}
	eng.fail = nil
	m.retryRemovals(context.Background())
	if s := lastSummary(t, w, "download_removed", id); !strings.Contains(s, "on a later try") {
		t.Fatalf("no History line for the retry: %q", s)
	}
	if d := lastData(t, w, "download_removed", id); d["retried"] != true {
		t.Fatalf("retry data %v", d)
	}
}
