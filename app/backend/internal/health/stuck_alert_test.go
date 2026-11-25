package health

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Downloader that answers while every download on it stopped together
// warns on its own check and, past the flap guard, reaches Alerts like a
// Downloader that does not answer; one moving download clears it again.
func TestStuckDownloaderReachesAlerts(t *testing.T) {
	w := newWorld(t)
	w.addDownloader(t, "qBittorrent", "ok")
	w.run(t)
	for _, id := range []string{"aaaa", "bbbb"} {
		w.exec(t, `INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, state, state_reason, files_seen, started_by)
			VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 1, '[]', ?, 'stalled', ?, '[]', '{}')`, id, stuckReason)
	}
	if c := w.run(t)["downloader:1"]; c.State != Warning {
		t.Fatalf("stuck Downloader %+v", c)
	}
	w.run(t)
	es := w.events(t)
	if len(es) != 1 || es[0].Kind != domain.EventHealthBroken || es[0].Data["check"] != "downloader:1" {
		t.Fatalf("stuck Downloader events %+v", es)
	}
	w.exec(t, "UPDATE downloads SET state = 'downloading', state_reason = NULL")
	w.run(t)
	w.run(t)
	if es = w.events(t); len(es) != 2 || es[0].Kind != domain.EventHealthRecovered {
		t.Fatalf("after a download moved %+v", es)
	}
}
