package api

import (
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
)

// A failed Usenet job says why; a finished one is Done, not Seeding; a
// read-only Downloader's rows offer no controls.
func TestClientRowUsenetWords(t *testing.T) {
	sab := domain.Downloader{ID: 77, Name: "S", Adapter: "sabnzbd", Protocol: "usenet"}
	failed := clientRow(sab, downloader.Job{ID: "a", State: downloader.StateFailed, Detail: "Aborted, cannot be completed"})
	if failed.Text != "Aborted, cannot be completed" {
		t.Fatalf("failed text %q", failed.Text)
	}
	done := clientRow(sab, downloader.Job{ID: "b", State: downloader.StateFinished, Progress: 1})
	if done.State == "seeding" || done.Phase != "done" {
		t.Fatalf("finished usenet %q/%q", done.State, done.Phase)
	}
	tor := clientRow(domain.Downloader{ID: 78, Protocol: "torrent"}, downloader.Job{ID: "c", State: downloader.StateFinished})
	if tor.State != "seeding" {
		t.Fatalf("torrent %q", tor.State)
	}
	if !done.CanAct {
		t.Fatal("writable downloader should act")
	}
	t.Setenv(downloader.ReadOnlyEnv, "77")
	if r := clientRow(sab, downloader.Job{ID: "d", State: downloader.StateDownloading}); r.CanAct {
		t.Fatal("read-only downloader row can act")
	}
}
