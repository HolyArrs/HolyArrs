package health

import (
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Imported downloads whose clear from the Downloader failed raise one
// warning while any still waits, in plain words with the last reason; the
// warning goes once the clears worked, so Alerts say it was fixed.
func TestClearWaitingCheck(t *testing.T) {
	w := newWorld(t)
	w.addDownloader(t, "SABnzbd", "ok")
	if c, ok := w.run(t)["clear_waiting"]; !ok || c.State != OK {
		t.Fatalf("nothing waits to be cleared: %+v (found %v)", c, ok)
	}
	insert := `INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, state, files_seen, started_by)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 1, '[]', ?, 'finished', '[]', '{}')`
	w.exec(t, insert, "SABnzbd_nzo_one")
	w.exec(t, "UPDATE downloads SET clear_retry_at = ?, clear_tries = 2, clear_error = 'SABnzbd answered 503' WHERE external_id = 'SABnzbd_nzo_one'",
		database.FormatTime(w.now.Add(10*time.Minute)))
	c := w.run(t)["clear_waiting"]
	if c.State != Warning || !strings.Contains(c.Message, "1 imported download") || !strings.Contains(c.Message, "SABnzbd answered 503") {
		t.Fatalf("one download waiting to be cleared: %+v", c)
	}
	if c.Kind != "clear_waiting" || c.Subject != "Downloads" {
		t.Fatalf("kind and subject: %+v", c)
	}
	w.exec(t, insert, "SABnzbd_nzo_two")
	w.exec(t, "UPDATE downloads SET clear_retry_at = ?, clear_tries = 1, clear_error = 'timeout' WHERE external_id = 'SABnzbd_nzo_two'",
		database.FormatTime(w.now.Add(time.Minute)))
	if c := w.run(t)["clear_waiting"]; c.State != Warning || !strings.Contains(c.Message, "2 imported downloads") {
		t.Fatalf("two downloads waiting to be cleared: %+v", c)
	}
	// Held for the flap guard's runs, it reaches Alerts like any other check.
	if es := w.events(t); len(es) != 1 || es[0].Kind != domain.EventHealthBroken || es[0].Data["check"] != "clear_waiting" {
		t.Fatalf("events after the warning held: %+v", es)
	}
	// Both clears worked: the warning goes, and Alerts hear it was fixed.
	w.exec(t, "UPDATE downloads SET clear_retry_at = NULL, clear_tries = 0, clear_error = ''")
	if c := w.run(t)["clear_waiting"]; c.State != OK || c.Message != "" {
		t.Fatalf("nothing waits any more, still %+v", c)
	}
	w.run(t)
	if es := w.events(t); len(es) != 2 || es[0].Kind != domain.EventHealthRecovered {
		t.Fatalf("events after the clears worked: %+v", es)
	}
}
