package sources

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ageRelease sets a Release's last change days ago.
func ageRelease(t *testing.T, f fixture, id int64, days int) {
	t.Helper()
	exec(t, f, "UPDATE releases SET updated_at = ? WHERE id = ?", database.FormatTime(time.Now().Add(-time.Duration(days)*24*time.Hour)), id)
}

func releaseExists(t *testing.T, f fixture, id int64) bool {
	t.Helper()
	var n int
	if err := f.s.Read().QueryRow("SELECT count(*) FROM releases WHERE id = ?", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// addSearchJob adds a finished search Job of the fixture's Slot.
func addSearchJob(t *testing.T, f fixture) int64 {
	t.Helper()
	var id int64
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by)
			VALUES ('t', '{}', 't', '{}', ?, json_object('type', 'slot', 'id', ?), '{}', 'normal', 'done', 1, 1, 't', '{"kind":"system"}')`, jobs.KindSearchSlot, f.slotID)
		if err == nil {
			id, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestPruneReleases_KeepsWhatIsNeeded seeds a Release for every case and
// checks that only old Releases nothing needs are removed.
func TestPruneReleases_KeepsWhatIsNeeded(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	cases := []struct {
		name  string
		state string
		days  int
		setup func(id int64)
		keep  bool
	}{
		{name: "old seen", state: "seen", days: 30, keep: false},
		{name: "old judged", state: "judged", days: 30, keep: false},
		{name: "old passed over", state: "passed_over", days: 15, keep: false},
		{name: "old refused", state: "refused", days: 400, keep: false},
		{name: "recent seen", state: "seen", days: 13, keep: true},
		{name: "old chosen", state: "chosen", days: 30, keep: true},
		{name: "old held", state: "held", days: 30, keep: true},
		{name: "old turned down", state: "turned_down", days: 30, keep: true},
		{name: "taken off turned down for one Slot", state: "passed_over", days: 30, keep: true, setup: func(id int64) {
			exec(t, f, `UPDATE releases SET verdict = json_object('turned_down_slot_ids', json_array(?)) WHERE id = ?`, f.slotID, id)
		}},
		{name: "old with a Download", state: "passed_over", days: 30, keep: true, setup: func(id int64) {
			if err := f.s.Write(ctx, func(tx *sql.Tx) error {
				d := &domain.Downloader{Name: fmt.Sprint("q", id), Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
				if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
					return err
				}
				dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &id, SlotIDs: []int64{}, ExternalID: "e", State: "seeding", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
				_, err := database.Insert(tx, "downloads", dl, domain.System)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "old with a send attempt", state: "passed_over", days: 30, keep: true, setup: func(id int64) {
			if err := f.s.Write(ctx, func(tx *sql.Tx) error {
				d := &domain.Downloader{Name: fmt.Sprint("n", id), Adapter: "sabnzbd", Protocol: "usenet", URL: "http://n", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
				if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
					return err
				}
				_, err := tx.Exec(`INSERT INTO downloader_attempts(downloader_id, release_id, slot_id, marker, category, state, created_at, updated_at)
					VALUES(?, ?, ?, ?, 'c', 'resolved', 't', 't')`, d.ID, id, f.slotID, fmt.Sprint("m", id))
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "old held by a Slot", state: "judged", days: 30, keep: true, setup: func(id int64) {
			exec(t, f, "UPDATE slots SET held_release_id = ? WHERE id = ?", id, f.slotID)
		}},
		{name: "old chosen by a Slot", state: "judged", days: 30, keep: true, setup: func(id int64) {
			exec(t, f, "UPDATE slots SET chosen_release_id = ? WHERE id = ?", id, f.slotID)
		}},
		{name: "old named in History", state: "passed_over", days: 30, keep: true, setup: func(id int64) {
			if err := f.s.AddEvent(ctx, &domain.Event{Actor: domain.System, Kind: EventReleaseTurnedDown, Summary: "x", Data: map[string]any{"release_id": id}}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	ids := make([]int64, len(cases))
	for i, c := range cases {
		ids[i] = addRelease(t, f, c.name, c.state)
		if c.setup != nil {
			c.setup(ids[i])
		}
		ageRelease(t, f, ids[i], c.days)
	}
	// An older search of the Slot goes, its newest search stays.
	oldJob, newJob := addSearchJob(t, f), addSearchJob(t, f)
	inOld, inNew := addRelease(t, f, "older search", "judged"), addRelease(t, f, "newest search", "judged")
	exec(t, f, "UPDATE releases SET job_id = ? WHERE id = ?", oldJob, inOld)
	exec(t, f, "UPDATE releases SET job_id = ? WHERE id = ?", newJob, inNew)
	ageRelease(t, f, inOld, 60)
	ageRelease(t, f, inNew, 60)

	n, err := PruneReleases(ctx, f.s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	removed := 0
	for i, c := range cases {
		if got := releaseExists(t, f, ids[i]); got != c.keep {
			t.Errorf("%s: kept %v, want %v", c.name, got, c.keep)
		}
		if !c.keep {
			removed++
		}
	}
	if releaseExists(t, f, inOld) || !releaseExists(t, f, inNew) {
		t.Errorf("search jobs: older kept %v, newest kept %v", releaseExists(t, f, inOld), releaseExists(t, f, inNew))
	}
	if n != removed+1 {
		t.Errorf("removed %d, want %d", n, removed+1)
	}
	// The Releases sheet still shows the newest search.
	got, err := latestSearchJobs(ctx, f.s.Read(), titleOfSlot(t, f), nil)
	if err != nil || len(got) != 1 || got[0].(int64) != newJob {
		t.Errorf("newest search after clean-up: %v %v", got, err)
	}
	// A second run finds nothing more.
	if n, err := PruneReleases(ctx, f.s, time.Now()); err != nil || n != 0 {
		t.Errorf("second run removed %d %v", n, err)
	}
}

func titleOfSlot(t *testing.T, f fixture) int64 {
	t.Helper()
	var id int64
	if err := f.s.Read().QueryRow("SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", f.slotID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestPruneReleases_LargeTableInBatches fills 300,000 old Releases and
// checks they all go in batches, with the newest ones kept.
func TestPruneReleases_LargeTableInBatches(t *testing.T) {
	if testing.Short() {
		t.Skip("large table")
	}
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	old := database.FormatTime(time.Now().Add(-40 * 24 * time.Hour))
	const total, recent = 300000, 1000
	for start := 0; start < total; start += 20000 {
		if err := f.s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`WITH RECURSIVE n(i) AS (SELECT ? UNION ALL SELECT i + 1 FROM n WHERE i + 1 < ?)
				INSERT INTO releases (created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol, found_by, state)
				SELECT ?, '{}', CASE WHEN i < ? THEN ? ELSE ? END, '{}', ?, 'r' || i, 'x', 'torrent', 'feed', 'judged' FROM n`,
				start, start+20000, old, total-recent, old, database.FormatTime(time.Now()), f.srcID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	before := f.s.WriteCount()
	began := time.Now()
	n, err := PruneReleases(ctx, f.s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != total-recent {
		t.Fatalf("removed %d, want %d", n, total-recent)
	}
	if writes := f.s.WriteCount() - before; writes < int64((total-recent)/releasePruneBatch) {
		t.Fatalf("%d writes: not in batches", writes)
	}
	var left int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM releases").Scan(&left)
	if left != recent {
		t.Fatalf("%d left, want %d", left, recent)
	}
	t.Logf("removed %d in %s", n, time.Since(began))
}
