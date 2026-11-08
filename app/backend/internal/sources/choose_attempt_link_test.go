package sources

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestTurnedDownRemoval_KeepsAttemptAuditLink checks that taking a Release off the Turned down list keeps the link from its attempt record.
func TestTurnedDownRemoval_KeepsAttemptAuditLink(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	id := addRelease(t, f, tv720, "turned_down")
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "Audit NZBGet", Adapter: "nzbget", Protocol: "usenet", URL: "http://nzbget", Category: "holyarrs", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		now := database.FormatTime(time.Now())
		_, err := tx.Exec(`INSERT INTO downloader_attempts(downloader_id, release_id, slot_id, marker, category, state, created_at, updated_at)
			VALUES(?, ?, ?, 'nzbget-<audit-fixture>', 'holyarrs', 'abandoned', ?, ?)`, d.ID, id, f.slotID, now, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := (&Searcher{Store: f.s}).RemoveTurnedDown(ctx, id, domain.System); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var state string
	if err := f.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", id).Scan(&state); err != nil || state != "passed_over" {
		t.Fatal(state, err)
	}
	if err := f.s.Read().QueryRow("SELECT state FROM downloader_attempts WHERE release_id = ?", id).Scan(&state); err != nil || state != "abandoned" {
		t.Fatal(state, err)
	}
}
