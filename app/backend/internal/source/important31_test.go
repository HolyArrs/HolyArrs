package source

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
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

// #41: a Release the Pick rules refused for safety, once forced, sent and
// turned down, stays refused when the Turned down list is cleared: picking
// it again still needs force (no one-click send of a program file).
func TestTurnedDownRemovalKeepsRefusedNeedingForce(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	exe := addRelease(t, f, "Dune.Part.Two.2024.1080p.WEB.H264-X.mkv.exe", "turned_down")
	exec(t, f, `UPDATE releases SET matched_slot_ids = json_array(?), verdict = json('{"outcome":"refused","passed":false,"reasons":[{"step":"hard","rule":"safety:executable","effect":"fail","detail":"contains a program file, not a video"}],"turned_down_reason":"dangerous files"}') WHERE id = ?`, f.slotID, exe)
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := store.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &exe, SlotIDs: []int64{f.slotID}, ExternalID: "e", State: "failed", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := store.Insert(tx, "downloads", dl, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	if n, err := sr.RemoveTurnedDown(ctx, 0, domain.System); err != nil || n != 1 {
		t.Fatalf("removed %d %v", n, err)
	}
	if got := relByName(t, f, "Dune.Part.Two.2024.1080p.WEB.H264-X.mkv.exe"); got.State != "refused" {
		t.Fatalf("after clearing the list: %s", got.State)
	}
	var nf *NeedsForceError
	if _, err := sr.Choose(ctx, exe, f.slotID, false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("picked again without force: %v", err)
	}
}

// #41: a refused Release sent once with force and then removed (not turned
// down) is still refused by its verdict: picking it again needs force.
func TestForcedRefusedReleaseNeedsForceAgain(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	exe := addRelease(t, f, "Dune.Part.Two.2024.1080p.WEB.H264-X.mkv.exe", "chosen")
	exec(t, f, `UPDATE releases SET matched_slot_ids = json_array(?), verdict = json('{"outcome":"refused","passed":false,"reasons":[{"step":"hard","rule":"safety:executable","effect":"fail","detail":"contains a program file, not a video"}]}') WHERE id = ?`, f.slotID, exe)
	var nf *NeedsForceError
	if _, err := (&Searcher{Store: f.s}).Choose(context.Background(), exe, f.slotID, false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("picked again without force: %v", err)
	}
}
