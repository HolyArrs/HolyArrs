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

// TestQueueAfterFailure_AlbumSearchesAlbumAgain checks that a failed album queues one album search for all its tracks, which takes the next Release.
func TestQueueAfterFailure_AlbumSearchesAlbumAgain(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac, mp3} }}
	f := seedArtist(t, ms.start(t))
	ctx := context.Background()
	var ids []int64
	for _, l := range ltaTracks {
		ids = append(ids, f.slots[l])
	}
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		r := &domain.Release{SourceID: f.srcID, Name: "Velvet Cartography - Low Tide Atlas (2019) [FLAC]", LinkRef: "http://src/0.torrent", Protocol: "torrent",
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: ids, Verdict: map[string]any{"turned_down_reason": "the download failed"},
			FoundBy: "auto_search", State: "turned_down"}
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		d := &domain.Download{DownloaderID: dr.ID, ReleaseID: &r.ID, SlotIDs: ids, ExternalID: "a", State: "failed", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", d, domain.System); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.Exec("UPDATE slots SET state = 'failed', active_download_id = ? WHERE id = ?", d.ID, id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	q := jobs.New(f.s, jobs.Options{})
	sr := &Searcher{Store: f.s, Queue: q}
	for _, id := range ids {
		if err := sr.QueueAfterFailure(ctx, id, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	var albums, others int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ?", jobs.KindSearchAlbum).Scan(&albums)
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind != ?", jobs.KindSearchAlbum).Scan(&others)
	if albums != 1 || others != 0 {
		t.Fatalf("queued %d album searches and %d other jobs", albums, others)
	}
	if err := f.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DELETE FROM jobs"); return err }); err != nil {
		t.Fatal(err)
	}
	var grabs [][2]int64
	sr = &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, releaseID int64) { grabs = append(grabs, [2]int64{slotID, releaseID}) }}
	if j := f.albumJob(t, sr, lta, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	other := relByName(t, f.fixture, "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]")
	if other.State != "chosen" || len(grabs) != 1 || grabs[0][1] != other.ID {
		t.Fatalf("next release %s, grabs %v", other.State, grabs)
	}
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Fatalf("%s: %s", l, st)
		}
	}
}
