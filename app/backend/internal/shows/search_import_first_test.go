package shows

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// A season whose pack is on disk but failed to import gets its import asked
// again, not a new search that fetches each episode on its own.
func TestSearchRetriesAPackImportBeforeSearching(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var ti mediafiles.Title
	if err := mediafiles.LoadTitle(ctx, s.Read(), id, &ti); err != nil {
		t.Fatal(err)
	}
	part, err := mediafiles.SeasonPartID(ctx, s.Read(), id, 1)
	if err != nil {
		t.Fatal(err)
	}
	pack := []int64{}
	rows, err := s.Read().QueryContext(ctx, "SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.part_id = ? ORDER BY s.id", part)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var sid int64
		if err := rows.Scan(&sid); err != nil {
			t.Fatal(err)
		}
		pack = append(pack, sid)
	}
	_ = rows.Close()
	if len(pack) < 2 {
		t.Fatalf("season 1 has %d slots", len(pack))
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	for _, sid := range pack {
		exec("UPDATE slots SET state = 'needs_a_decision', last_reason = 'reading the download folder took too long; try again' WHERE id = ?", sid)
	}
	var asked []int64
	retryAll := func(_ context.Context, ids []int64, _ domain.Actor) map[int64]bool {
		asked = append(asked, ids...)
		out := map[int64]bool{}
		for _, id := range ids {
			out[id] = true
		}
		return out
	}
	h := &SearchHandlers{Store: s, Queue: svc.Queue, RetryImport: retryAll}
	season := 1
	searches := func() int {
		var n int
		_ = s.Read().QueryRow("SELECT COUNT(*) FROM jobs WHERE kind IN ('search_season', 'search_slot')").Scan(&n)
		return n
	}
	call := func(in sources.TitleSearchIn) {
		rec := httptest.NewRecorder()
		h.Search(rec, httptest.NewRequest(http.MethodPost, "/", nil), ti, in)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	}
	// No download linked: nothing to retry, the season is searched.
	call(sources.TitleSearchIn{Season: &season})
	if len(asked) != 0 || searches() != 1 {
		t.Fatalf("asked %v, searches %d", asked, searches())
	}
	exec("DELETE FROM jobs")
	exec("INSERT INTO downloaders (id, created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES (7, 'a', '{}', 'a', '{}', 'qb', 'qbittorrent', 'torrent', 'http://x')")
	exec("INSERT INTO downloads (id, created_at, created_by, updated_at, updated_by, downloader_id, external_id, progress, state, started_by) VALUES (900, 'a', '{}', 'a', '{}', 7, 'pack', 1, 'finished', '{}')")
	for _, sid := range pack {
		exec("UPDATE slots SET active_download_id = 900 WHERE id = ?", sid)
	}
	call(sources.TitleSearchIn{Season: &season})
	slices.Sort(asked)
	if !slices.Equal(asked, pack) || searches() != 0 {
		t.Fatalf("asked %v, want %v; searches %d", asked, pack, searches())
	}
	asked = nil
	call(sources.TitleSearchIn{SlotID: &pack[0]})
	if len(asked) != 1 || asked[0] != pack[0] || searches() != 0 {
		t.Fatalf("one episode: asked %v, searches %d", asked, searches())
	}
}
