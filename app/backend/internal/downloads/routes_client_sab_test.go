package downloads

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A failed Usenet job says why; a finished one is Done, not Seeding; a
// read-only Downloader's rows offer no controls.
func TestClientRowUsenetWords(t *testing.T) {
	sab := domain.Downloader{Writable: true, ID: 77, Name: "S", Adapter: "sabnzbd", Protocol: "usenet"}
	failed := clientRow(sab, Job{ID: "a", State: StateFailed, Detail: "Aborted, cannot be completed"})
	if failed.Text != "Aborted, cannot be completed" {
		t.Fatalf("failed text %q", failed.Text)
	}
	done := clientRow(sab, Job{ID: "b", State: StateFinished, Progress: 1})
	if done.State == "seeding" || done.Phase != "done" {
		t.Fatalf("finished usenet %q/%q", done.State, done.Phase)
	}
	tor := clientRow(domain.Downloader{Writable: true, ID: 78, Protocol: "torrent"}, Job{ID: "c", State: StateFinished})
	if tor.State != "seeding" {
		t.Fatalf("torrent %q", tor.State)
	}
	if !done.CanAct {
		t.Fatal("writable downloader should act")
	}
	t.Setenv(ReadOnlyEnv, "77")
	if r := clientRow(sab, Job{ID: "d", State: StateDownloading}); r.CanAct {
		t.Fatal("read-only downloader row can act")
	}
}
