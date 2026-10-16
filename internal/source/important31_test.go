package source

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// I1: a Release chosen from a Source and not yet sent counts against the
// Source's picks limit.
func TestPicksLimitCountsChosenNotYetSent(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	exec(t, f, `UPDATE sources SET limits = '{"grabs_per_window":1,"window_seconds":3600}'`)
	addRelease(t, f, "Other.Movie.2024.1080p.WEB-DL-X", "chosen")
	runKind(t, f, &Searcher{Store: f.s}, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	if got := relByName(t, f, web1080); got.State != "passed_over" {
		t.Fatalf("release %s: the chosen one was not counted", got.State)
	}
}

// I1: taking a Release that has a Download off the Turned down list keeps
// the row, so the Download still counts for its Source.
func TestTurnedDownRemovalKeepsDownloadLink(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	id := addRelease(t, f, tv720, "turned_down")
	plain := addRelease(t, f, br1080, "turned_down")
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := store.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &id, SlotIDs: []int64{}, ExternalID: "e", State: "downloading", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := store.Insert(tx, "downloads", dl, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	if n, err := sr.RemoveTurnedDown(ctx, 0, domain.System); err != nil || n != 2 {
		t.Fatalf("removed %d %v", n, err)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM downloads WHERE release_id = ?", id).Scan(&n)
	if n != 1 {
		t.Fatal("the Download lost its Release")
	}
	_ = f.s.Read().QueryRow("SELECT count(*) FROM releases WHERE id = ?", plain).Scan(&n)
	if n != 0 {
		t.Fatal("a Release without a Download should be deleted")
	}
	if tds, _ := sr.turnedDown(ctx, f.slotID); len(tds) != 0 {
		t.Fatalf("still turned down: %v", tds)
	}
}

// I2: a feed Release that fits two Slots is stored once, with both Slots.
func TestFeedStoresReleaseOncePerRun(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	// A second Version of the same Title, with its own wanted Slot.
	exec(t, f, `INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, keep_up_to_date, is_main)
		SELECT created_at, created_by, updated_at, updated_by, title_id, '4K', pick_rules_id, library_folder_id, 'Dune 4K', 1, 0 FROM versions LIMIT 1`)
	exec(t, f, `INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state)
		SELECT created_at, created_by, updated_at, updated_by, item_id, (SELECT max(id) FROM versions), 'wanted' FROM slots LIMIT 1`)
	runKind(t, f, &Searcher{Store: f.s}, KindFeedCheck, nil, nil)
	rs, err := store.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080)
	if err != nil || len(rs) != 1 {
		t.Fatalf("stored %d times: %v", len(rs), err)
	}
	if len(rs[0].MatchedSlotIDs) != 2 {
		t.Fatalf("matched %v", rs[0].MatchedSlotIDs)
	}
	if bs, _ := rs[0].Verdict["by_slot"].(map[string]any); len(bs) != 2 {
		t.Fatalf("verdict per Slot missing: %v", strings.Join(nil, ""))
	}
}
