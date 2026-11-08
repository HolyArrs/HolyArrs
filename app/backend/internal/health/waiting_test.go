package health

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// waiting finds the check about downloads that wait because their
// Downloader can't use the files (late-mounts-3). Its key is the fix's
// choice; it is found by its plain words.
func waiting(cs map[string]domain.HealthCheck) (domain.HealthCheck, bool) {
	for _, c := range cs {
		if strings.Contains(c.Message, "can't use the files") {
			return c, true
		}
	}
	return domain.HealthCheck{}, false
}

func (w *world) addWaitingDownload(t *testing.T, since time.Time) int64 {
	t.Helper()
	reason := "the Downloader can't use the files of this download: is the share mounted?"
	d := &domain.Download{DownloaderID: 1, ExternalID: "29be7bfa8759643f2d5dac9f94d516c3378696ed", State: "paused", StateReason: &reason,
		SlotIDs: []int64{}, FilesSeen: []domain.SeenFile{}}
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "downloads", d, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE downloads SET updated_at = ? WHERE id = ?", database.FormatTime(since), d.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return d.ID
}

// Downloads waiting for their files are counted in plain words: a warning
// at first, an error once they have waited about 3 days. A download paused
// by a person is not counted.
func TestWaitingDownloadsCheck(t *testing.T) {
	w := newWorld(t)
	w.addDownloader(t, "qBittorrent", "ok")
	// Paused by a person (no reason): nothing to say.
	w.exec(t, `INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, state, files_seen, started_by)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 1, '[]', 'aaaa', 'paused', '[]', '{}')`)
	if c, ok := waiting(w.run(t)); ok && c.State != OK {
		t.Fatalf("a download paused by a person raised %+v", c)
	}
	id := w.addWaitingDownload(t, w.now.Add(-time.Hour))
	c, ok := waiting(w.run(t))
	if !ok || c.State != Warning || !strings.Contains(c.Message, "1 download") || !strings.Contains(c.Message, "recheck") {
		t.Fatalf("one waiting download: %+v (found %v)", c, ok)
	}
	w.exec(t, "UPDATE downloads SET updated_at = ? WHERE id = ?", database.FormatTime(w.now.Add(-4*24*time.Hour)), id)
	if c, ok := waiting(w.run(t)); !ok || c.State != Error {
		t.Fatalf("waiting 4 days: %+v (found %v), want error", c, ok)
	}
	// One download is "it", more are "them".
	if !strings.Contains(c.Message, "recheck it in") {
		t.Fatalf("one waiting download should read \"recheck it\": %q", c.Message)
	}
	w.exec(t, `INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, state, state_reason, files_seen, started_by)
		VALUES ('2026-10-01T00:00:00Z', '{}', ?, '{}', 1, '[]', 'bbbb', 'paused', 'the Downloader can''t use the files of this download: is the share mounted?', '[]', '{}')`, database.FormatTime(w.now.Add(-time.Hour)))
	if c, ok := waiting(w.run(t)); !ok || !strings.Contains(c.Message, "2 downloads") || !strings.Contains(c.Message, "recheck them in") {
		t.Fatalf("two waiting downloads should read \"recheck them\": %+v (found %v)", c, ok)
	}
	w.exec(t, "DELETE FROM downloads WHERE external_id = 'bbbb'")
	// The files came back.
	w.exec(t, "UPDATE downloads SET state = 'downloading', state_reason = NULL WHERE id = ?", id)
	if c, ok := waiting(w.run(t)); ok && c.State != OK {
		t.Fatalf("nothing waits any more, still %+v", c)
	}
}
