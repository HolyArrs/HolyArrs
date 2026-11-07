package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/dlsync"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// noClient is a reader Manager that never reaches a client: the store only
// holds what the test loads.
type noClient struct{ m *downloader.Manager }

func (noClient) Reader(context.Context, domain.Downloader) (downloader.Engine, uint64, error) {
	return nil, 0, errors.New("no client in this test")
}
func (noClient) ReportRead(context.Context, domain.Downloader, error) {}
func (noClient) Generation(int64) uint64                              { return 0 }
func (n noClient) Interval(a int) time.Duration                       { return n.m.Interval(a) }

// The view reads the reader's snapshot only: no
// client call, no database write, no matcher start; it carries the revs
// and pages outside rows as before.
func TestDownloadsViewFromStore(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	var calls atomic.Int64
	qb := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	t.Cleanup(qb.Close)
	pointQB(t, x, qb.URL)
	var dlr int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&dlr)
	exec41(t, x.s, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('links', 'downloads_sync', 'on', '', '{}', '', '{}')`)

	sy := &dlsync.Sync{Store: &dlsync.Store{}, DB: x.s, Manager: noClient{x.api.manager()}}
	x.api.s.DLSync = sy
	var jobs []downloader.Job
	for i := range 250 {
		jobs = append(jobs, downloader.Job{ID: fmt.Sprintf("%040x", i+1), Name: fmt.Sprintf("Outside.%d", i), Category: "linux", Size: 100, Progress: 1,
			UpSpeed: 10, State: downloader.StateFinished, Detail: "uploading", AddedAt: int64(1700000000 + i), Ratio: 1.5, LastActivity: int64(1700100000 + i)})
	}
	sy.Store.Load(dlr, jobs)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sy.Around(ctx, func(ctx context.Context) { <-ctx.Done() }); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	for !sy.On() {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // let the reader's first (failed) read pass

	conn, err := x.s.Read().Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	version := func() int64 {
		var v int64
		if err := conn.QueryRowContext(context.Background(), "PRAGMA data_version").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	calls.Store(0)
	before := version()
	var first struct {
		viewAnswer
		Revs map[string]uint64 `json:"revs"`
	}
	start := time.Now()
	for i := range 5 {
		b := do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200)
		if i == 0 {
			if err := json.Unmarshal(b, &first); err != nil {
				t.Fatal(err)
			}
		}
	}
	if took := time.Since(start) / 5; took > 300*time.Millisecond {
		t.Fatalf("a view took %v", took)
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("the view made %d client calls", n)
	}
	if after := version(); after != before {
		t.Fatalf("the view wrote to the database (data_version %d -> %d)", before, after)
	}
	if n := count(t, x.s, "SELECT COUNT(*) FROM dl_match_queue"); n != 0 {
		t.Fatalf("the view queued %d matches", n)
	}
	if first.OutsideTotal != 250 || first.NextCursor != "200" || len(first.Downloads) != 201 {
		t.Fatalf("outside %d cursor %q rows %d", first.OutsideTotal, first.NextCursor, len(first.Downloads))
	}
	if first.Revs[fmt.Sprint(dlr)] == 0 {
		t.Fatalf("revs %v", first.Revs)
	}
	if len(first.Clients) != 1 || first.Clients[0].Health != "checking" || !first.Clients[0].Stale {
		t.Fatalf("client health %+v, want checking (last known rows)", first.Clients)
	}
	if first.Totals.UpSpeed != 0 {
		// Last-known rows carry no speeds: they are not current.
		t.Fatalf("upload total %d, want 0", first.Totals.UpSpeed)
	}
	if !sy.Store.Viewer() {
		t.Fatal("a view did not count as a viewer")
	}
	// The next page from the cursor.
	v := view(t, x.srv.URL+"/api/v1/downloads/view?cursor=200")
	if len(v.Downloads) != 51 || v.NextCursor != "" {
		t.Fatalf("page 2: rows %d cursor %q", len(v.Downloads), v.NextCursor)
	}
	// One client's page only (a rev gap on that client): its rows and its rev.
	var one struct {
		viewAnswer
		Revs map[string]uint64 `json:"revs"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+fmt.Sprintf("/api/v1/downloads/view?client=%d", dlr), "", 200), &one)
	if len(one.Downloads) != 201 || len(one.Revs) != 1 || one.Revs[fmt.Sprint(dlr)] != first.Revs[fmt.Sprint(dlr)] {
		t.Fatalf("client page: rows %d revs %v", len(one.Downloads), one.Revs)
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view?client=999999", "", 200), &one)
	if len(one.Downloads) != 0 {
		t.Fatalf("other client page: rows %d, want 0", len(one.Downloads))
	}
	// The torrent detail's last activity comes from the reader's snapshot,
	// not the empty list cache.
	var td downloader.TorrentDetail
	x.api.torrentTimeline(context.Background(), dlr, fmt.Sprintf("%040X", 3), &td, nil)
	if td.LastActivity == nil || td.LastActivity.Unix() != 1700100002 {
		t.Fatalf("last activity %v, want from the reader", td.LastActivity)
	}
	// The health page names the client the reader is checking again.
	var hc struct {
		Sync []ClientHealth `json:"sync"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/health/checks", "", 200), &hc)
	if len(hc.Sync) != 1 || hc.Sync[0].ID != dlr || hc.Sync[0].Health != "checking" {
		t.Fatalf("health sync %+v, want checking", hc.Sync)
	}
	sy.Store.SetFallback(dlr, true)
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/health/checks", "", 200), &hc)
	if len(hc.Sync) != 1 || hc.Sync[0].Health != "degraded" {
		t.Fatalf("health sync %+v, want degraded", hc.Sync)
	}
}

// Link-only matching never adds a title: a sure match missing from the
// Library is only suggested.
func TestMatchLinkOnlyNeverAdds(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	added := 0
	x.api.match.addMovie = func(context.Context, string) (int64, error) { added++; return 0, nil }
	x.api.match.addShow = func(context.Context, string) (int64, error) { added++; return 0, nil }
	var titles int
	_ = x.s.Read().QueryRow("SELECT COUNT(*) FROM titles").Scan(&titles)
	out, tid, err := x.api.MatchLinkOnly(context.Background(), 1, "ABCDEF0000000000000000000000000000000000", "Some.Linux.ISO.x64", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if out != linkNotMedia || tid != 0 {
		t.Fatalf("outcome %q %d", out, tid)
	}
	var after int
	_ = x.s.Read().QueryRow("SELECT COUNT(*) FROM titles").Scan(&after)
	if added != 0 || after != titles {
		t.Fatalf("titles %d -> %d, adds %d", titles, after, added)
	}
	var st string
	if err := x.s.Read().QueryRow("SELECT state FROM client_links WHERE hash = 'abcdef0000000000000000000000000000000000'").Scan(&st); err != nil || st != linkNotMedia {
		t.Fatalf("link %q %v", st, err)
	}
	// A sure match not in the Library: suggested, still no add.
	x.api.match.searchMovies = func(context.Context, string, int) ([]metadata.Movie, error) {
		return []metadata.Movie{{Name: "Arrival", Year: 2016, IDs: map[string]string{"tmdb": "329865"}}}, nil
	}
	out, tid, err = x.api.MatchLinkOnly(context.Background(), 1, "bbbb000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != "suggested" || tid != 0 {
		t.Fatalf("sure match not in the Library: %q %d %v", out, tid, err)
	}
	_ = x.s.Read().QueryRow("SELECT COUNT(*) FROM titles").Scan(&after)
	if added != 0 || after != titles {
		t.Fatalf("suggested match added a title: titles %d -> %d, adds %d", titles, after, added)
	}
	_ = sql.ErrNoRows
}

// downloads_auto_add on: a sure match not in the Library is added (once,
// even when matched again after a restart) and linked, with a log line;
// not-media and unsure names never add.
func TestMatchAutoAdd(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	ctx := context.Background()
	exec41(t, x.s, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('links', 'downloads_auto_add', 'true', '', '{}', '', '{}')`)
	var adds int
	var newID int64
	// The add stands in for the real one: the seeded title takes the
	// looked-up id, so the Library holds it from then on.
	_ = x.s.Read().QueryRow("SELECT id FROM titles ORDER BY id LIMIT 1").Scan(&newID)
	exec41(t, x.s, `UPDATE titles SET source_ids = '{}', kind = 'movie'`)
	x.api.match.addMovie = func(ctx context.Context, id string) (int64, error) {
		adds++
		exec41(t, x.s, `UPDATE titles SET source_ids = '{"tmdb":"329865"}' WHERE id = `+fmt.Sprint(newID))
		return newID, nil
	}
	x.api.match.searchMovies = func(context.Context, string, int) ([]metadata.Movie, error) {
		return []metadata.Movie{{Name: "Arrival", Year: 2016, IDs: map[string]string{"tmdb": "329865"}}}, nil
	}
	if out, _, err := x.api.MatchLinkOnly(ctx, 1, "cccc000000000000000000000000000000000000", "Some.Linux.ISO.x64", "linux"); err != nil || out != linkNotMedia {
		t.Fatalf("not media: %q %v", out, err)
	}
	if out, _, err := x.api.MatchLinkOnly(ctx, 1, "dddd000000000000000000000000000000000000", "Arival.Two.2016.1080p", "movies"); err != nil || out != linkUnsure {
		t.Fatalf("unsure: %q %v", out, err)
	}
	if adds != 0 {
		t.Fatalf("adds %d before a sure match", adds)
	}
	out, tid, err := x.api.MatchLinkOnly(ctx, 1, "bbbb000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != linkLinked || tid == 0 || tid != newID || adds != 1 {
		t.Fatalf("auto add: %q %d %v adds %d", out, tid, err, adds)
	}
	var line string
	_ = x.s.Read().QueryRow("SELECT summary FROM torrent_log WHERE hash = 'bbbb000000000000000000000000000000000000'").Scan(&line)
	if line != "Added to your Library" {
		t.Fatalf("log line %q", line)
	}
	// A restart: the link row is gone (as if lost) but the title stays;
	// matching again links the same title and adds nothing.
	exec41(t, x.s, `DELETE FROM client_links`)
	out, tid2, err := x.api.MatchLinkOnly(ctx, 1, "bbbb000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != linkLinked || tid2 != tid || adds != 1 {
		t.Fatalf("again: %q %d %v adds %d", out, tid2, err, adds)
	}
	// Off: a new sure match is only suggested.
	exec41(t, x.s, `UPDATE settings SET value = 'false' WHERE key = 'downloads_auto_add'`)
	exec41(t, x.s, `DELETE FROM client_links`)
	exec41(t, x.s, `UPDATE titles SET source_ids = '{}'`)
	out, _, err = x.api.MatchLinkOnly(ctx, 1, "eeee000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != "suggested" || adds != 1 {
		t.Fatalf("off: %q %v adds %d", out, err, adds)
	}
	// On again: the matcher's own suggestion is added; a person's choice
	// (any other how) stays as it is.
	exec41(t, x.s, `INSERT INTO client_links (downloader_id, hash, name, state, slot_ids, covers, candidates, how, matched_at) VALUES (1, 'ffff000000000000000000000000000000000000', 'Arrival.2016.1080p', 'unsure', '[]', '', '[]', 'rejected', '')`)
	exec41(t, x.s, `UPDATE settings SET value = 'true' WHERE key = 'downloads_auto_add'`)
	out, tid, err = x.api.MatchLinkOnly(ctx, 1, "eeee000000000000000000000000000000000000", "Arrival.2016.1080p.BluRay.x264-GRP", "movies")
	if err != nil || out != linkLinked || tid != newID || adds != 2 {
		t.Fatalf("upgrade: %q %d %v adds %d", out, tid, err, adds)
	}
	out, _, err = x.api.MatchLinkOnly(ctx, 1, "ffff000000000000000000000000000000000000", "Arrival.2016.1080p", "movies")
	if err != nil || out != linkUnsure || adds != 2 {
		t.Fatalf("person choice changed: %q %v adds %d", out, err, adds)
	}
	// Match now with auto-add on puts suggested items back in the queue.
	exec41(t, x.s, `INSERT INTO dl_match_queue (downloader_id, key, name, category, priority, next_try_at, outcome) VALUES (1, 'gggg', 'X.2016', 'movies', 0, '', 'suggested')`)
	if n := x.api.rematch(ctx); n != 1 {
		t.Fatalf("rematch %d", n)
	}
	// A not-media call made by the name reading is looked at again; a
	// person's not-media pick is not.
	exec41(t, x.s, `INSERT INTO client_links (downloader_id, hash, name, state, slot_ids, covers, candidates, how, matched_at) VALUES (1, 'hhhh', 'Arrival_2016_Bluray-1080p', 'not_media', '[]', '', '[]', 'name', ''), (1, 'iiii', 'Arrival_2016_Bluray-1080p', 'not_media', '[]', '', '[]', 'by hand', '')`)
	if out, _, err := x.api.MatchLinkOnly(ctx, 1, "hhhh", "Arrival_2016_Bluray-1080p", "prowlarr"); err != nil || out != linkLinked {
		t.Fatalf("stale not media: %q %v", out, err)
	}
	if out, _, err := x.api.MatchLinkOnly(ctx, 1, "iiii", "Arrival_2016_Bluray-1080p", "prowlarr"); err != nil || out != linkNotMedia {
		t.Fatalf("person not media changed: %q %v", out, err)
	}
}
