package sources

import (
	"context"
	"database/sql"
	"net/url"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestQueueAfterFailure_PackSearchesSeasonAgain checks that a failed season pack is turned down for every episode and the season is searched once for all its Slots.
// A failed single episode is searched alone.
func TestQueueAfterFailure_PackSearchesSeasonAgain(t *testing.T) {
	failed := "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{failed, "Copper.Valley.S01.1080p.WEB-DL.H.264-OTHER", "Copper.Valley.S02E01.1080p.WEB-DL.H.264-QUILL"}
	}}
	f := seedShow(t, ts.start(t), func(label string, sl *mediafiles.Slot) {
		if label[:3] == "S01" || label == "S02E01" {
			sl.State = mediafiles.SlotFailed
		}
	})
	ctx := context.Background()
	pack := []int64{f.slots["S01E01"], f.slots["S01E02"], f.slots["S01E03"]}
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		r := &domain.Release{SourceID: f.srcID, Name: failed, LinkRef: "http://x/1", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: pack, Verdict: map[string]any{"turned_down_reason": "the download failed"}, FoundBy: "auto_search", State: "turned_down"}
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		for n, ids := range [][]int64{pack, {f.slots["S02E01"]}} {
			d := &domain.Download{DownloaderID: dr.ID, ReleaseID: &r.ID, SlotIDs: ids, ExternalID: string(rune('a' + n)), State: "failed", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
			if _, err := database.Insert(tx, "downloads", d, domain.System); err != nil {
				return err
			}
			for _, id := range ids {
				if _, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", d.ID, id); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	q := jobs.New(f.s, jobs.Options{})
	sr := &Searcher{Store: f.s, Queue: q}
	for _, id := range append(pack, f.slots["S02E01"]) {
		if err := sr.QueueAfterFailure(ctx, id, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	kinds := map[string]int{}
	rows, err := f.s.Read().Query("SELECT kind FROM jobs")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		kinds[k]++
	}
	rows.Close()
	if kinds[jobs.KindSearchSeason] != 1 || kinds[jobs.KindSearchSlot] != 1 {
		t.Fatalf("queued %v", kinds)
	}
	if err := f.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DELETE FROM jobs"); return err }); err != nil {
		t.Fatal(err)
	}
	var grabs [][2]int64
	sr = &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, releaseID int64) { grabs = append(grabs, [2]int64{slotID, releaseID}) }}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	other := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-OTHER")
	if other.State != "chosen" || len(PickSlots(other)) != 3 || len(grabs) != 1 || grabs[0][1] != other.ID {
		t.Fatalf("other pack %s %v, grabs %v, result %+v", other.State, PickSlots(other), grabs, j.Result)
	}
}
