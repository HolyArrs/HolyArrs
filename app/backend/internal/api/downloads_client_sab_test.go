package api

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// A failed Usenet job says why; a finished one is Done, not Seeding; a
// read-only Downloader's rows offer no controls.
func TestClientRowUsenetWords(t *testing.T) {
	sab := domain.Downloader{Writable: true, ID: 77, Name: "S", Adapter: "sabnzbd", Protocol: "usenet"}
	failed := clientRow(sab, downloads.Job{ID: "a", State: downloads.StateFailed, Detail: "Aborted, cannot be completed"})
	if failed.Text != "Aborted, cannot be completed" {
		t.Fatalf("failed text %q", failed.Text)
	}
	done := clientRow(sab, downloads.Job{ID: "b", State: downloads.StateFinished, Progress: 1})
	if done.State == "seeding" || done.Phase != "done" {
		t.Fatalf("finished usenet %q/%q", done.State, done.Phase)
	}
	tor := clientRow(domain.Downloader{Writable: true, ID: 78, Protocol: "torrent"}, downloads.Job{ID: "c", State: downloads.StateFinished})
	if tor.State != "seeding" {
		t.Fatalf("torrent %q", tor.State)
	}
	if !done.CanAct {
		t.Fatal("writable downloader should act")
	}
	t.Setenv(downloads.ReadOnlyEnv, "77")
	if r := clientRow(sab, downloads.Job{ID: "d", State: downloads.StateDownloading}); r.CanAct {
		t.Fatal("read-only downloader row can act")
	}
}
