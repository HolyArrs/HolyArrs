package downloads

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// countDriver wraps the SQLite driver and counts every query it runs.
type countDriver struct {
	driver.Driver
	n *atomic.Int64
}

func (d countDriver) Open(name string) (driver.Conn, error) {
	c, err := d.Driver.Open(name)
	if err != nil {
		return nil, err
	}
	return countConn{c, d.n}, nil
}

type countConn struct {
	driver.Conn
	n *atomic.Int64
}

func (c countConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	c.n.Add(1)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}

func (c countConn) CheckNamedValue(v *driver.NamedValue) error {
	if ch, ok := c.Conn.(driver.NamedValueChecker); ok {
		return ch.CheckNamedValue(v)
	}
	return driver.ErrSkip
}

var (
	countOnce  sync.Once
	countReads atomic.Int64
)

// countingPool opens a read pool on s's main file whose queries are counted.
func countingPool(t *testing.T, s *database.Store) *sql.DB {
	t.Helper()
	countOnce.Do(func() {
		base, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		sql.Register("sqlite-count", countDriver{base.Driver(), &countReads})
		_ = base.Close()
	})
	db, err := sql.Open("sqlite-count", "file:"+filepath.ToSlash(filepath.Join(s.Dir(), database.MainFile))+"?_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// qbList is a fake qBittorrent that lists the given torrents.
func qbList(t *testing.T, torrents string) *httptest.Server {
	t.Helper()
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch strings.TrimPrefix(r.URL.Path, "/api/v2/") {
		case "torrents/info":
			if r.Form.Get("category") != "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(torrents))
		case "app/webapiVersion":
			_, _ = w.Write([]byte("2.15.1"))
		}
	}))
	t.Cleanup(qb.Close)
	return qb
}

// outsideTorrents is n seeding outside torrents, each sending up bytes/s.
func outsideTorrents(n int, up int64) string {
	parts := make([]string, n)
	for i := range n {
		parts[i] = fmt.Sprintf(`{"hash":"%040x","name":"Outside.%d","category":"linux","size":100,"progress":1,"upspeed":%d,"state":"uploading","added_on":%d}`, i+1, i, up, 1700000000+i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// pointQB moves the seeded Downloader to url and drops its cached engine.
func pointQB(t *testing.T, x *dl41, url string) {
	t.Helper()
	exec41(t, x.s, "UPDATE downloaders SET url = ?", url)
	var id int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&id)
	x.dl.Manager().Forget(id)
}

type viewAnswer struct {
	Downloads    []DownloadRow  `json:"downloads"`
	Totals       DownloadTotals `json:"totals"`
	Clients      []ClientHealth `json:"clients"`
	OutsideTotal int            `json:"outside_total"`
	NextCursor   string         `json:"next_cursor"`
}

func view(t *testing.T, url string) viewAnswer {
	t.Helper()
	var v viewAnswer
	if err := json.Unmarshal(do(t, "GET", url, "", 200), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// A client that never answers does not hold the view up (the view never
// calls a client): every answer comes in under 300 ms and the client is
// marked as not read yet.
func TestDownloadsViewHungClient(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	stop := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	t.Cleanup(hang.Close)
	t.Cleanup(func() { close(stop) })
	pointQB(t, x, hang.URL)
	x.dl.DLSync = &Sync{Store: &Store{}, DB: x.s}
	var worst time.Duration
	var last viewAnswer
	for range 20 {
		at := time.Now()
		last = view(t, x.srv.URL+"/api/v1/downloads/view")
		worst = max(worst, time.Since(at))
	}
	t.Logf("slowest of 20 views: %v", worst)
	if worst >= 300*time.Millisecond {
		t.Fatalf("slowest view %v with a hung client", worst)
	}
	if len(last.Downloads) != 1 || last.Downloads[0].Title != "Salt Choir" {
		t.Fatalf("own rows %+v", last.Downloads)
	}
	if len(last.Clients) != 1 || last.Clients[0].Health == "ok" {
		t.Fatalf("health %+v", last.Clients)
	}
}

// A client that answered once and then stops shows its last list, marked
// stale, instead of an empty client.
func TestDownloadsViewStaleClient(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	pointQB(t, x, qbList(t, outsideTorrents(3, 0)).URL)
	feedStore(t, x.dl)
	if v := view(t, x.srv.URL+"/api/v1/downloads/view"); v.OutsideTotal != 3 || v.Clients[0].Health != "ok" {
		t.Fatalf("first %+v", v.Clients)
	}
	pointQB(t, x, "http://127.0.0.1:1")
	feedStore(t, x.dl) // the background read fails
	v := view(t, x.srv.URL+"/api/v1/downloads/view")
	if v.Clients[0].Health != "offline" || !v.Clients[0].Stale || v.OutsideTotal != 3 {
		t.Fatalf("after %+v outside %d", v.Clients, v.OutsideTotal)
	}
}

// The view reads each related table once, however many rows it shows.
func TestDownloadsViewQueryCount(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
		var d domain.Download
		if err := tx.QueryRow("SELECT downloader_id FROM downloads WHERE id = ?", x.dlID).Scan(&d.DownloaderID); err != nil {
			return err
		}
		for i := range 99 {
			n := &domain.Download{DownloaderID: d.DownloaderID, ReleaseID: &x.relID, SlotIDs: []int64{x.slotID}, ExternalID: fmt.Sprintf("%040x", 0xabc000+i),
				State: "downloading", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
			if _, err := database.Insert(tx, "downloads", n, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pointQB(t, x, qbList(t, `[]`).URL)
	feedStore(t, x.dl)
	x.dl.ViewDB = countingPool(t, x.s)
	view(t, x.srv.URL+"/api/v1/downloads/view")
	before := countReads.Load()
	v := view(t, x.srv.URL+"/api/v1/downloads/view")
	n := countReads.Load() - before
	t.Logf("%d queries for %d rows", n, len(v.Downloads))
	if n > 7 {
		t.Fatalf("%d queries for %d rows", n, len(v.Downloads))
	}
	if len(v.Downloads) != 100 || v.Downloads[50].Title != "Salt Choir" || v.Downloads[50].Version != "HD" {
		t.Fatalf("rows %d %+v", len(v.Downloads), v.Downloads[50])
	}

	// With outside rows, the count does not grow with how many there are.
	count := func(n int) int64 {
		pointQB(t, x, qbList(t, outsideTorrents(n, 0)).URL)
		feedStore(t, x.dl)
		view(t, x.srv.URL+"/api/v1/downloads/view")
		b := countReads.Load()
		if got := view(t, x.srv.URL+"/api/v1/downloads/view"); got.OutsideTotal != n {
			t.Fatalf("outside %d, want %d", got.OutsideTotal, n)
		}
		return countReads.Load() - b
	}
	few, many := count(10), count(300)
	t.Logf("with outside rows: %d queries (10 rows), %d (300 rows)", few, many)
	if few != many {
		t.Fatalf("queries %d with 10 outside rows, %d with 300", few, many)
	}
}

// Upload totals count every row that sends, seeding torrents included, and
// cover outside rows past the page.
func TestDownloadsViewTotalsIncludeSeeding(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "seeding")
	exec41(t, x.s, "UPDATE downloads SET up_speed = 100, speed = 0")
	pointQB(t, x, qbList(t, outsideTorrents(250, 10)).URL)
	feedStore(t, x.dl)
	v := view(t, x.srv.URL+"/api/v1/downloads/view")
	if v.Totals.UpSpeed != 100+250*10 || v.Totals.Speed != 0 {
		t.Fatalf("totals %+v", v.Totals)
	}
}

// Outside rows come 200 at a time with a cursor to the next page.
func TestDownloadsViewPagesOutsideRows(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	pointQB(t, x, qbList(t, outsideTorrents(450, 0)).URL)
	feedStore(t, x.dl)
	seen := map[string]bool{}
	cursor, pages := "", 0
	for {
		v := view(t, x.srv.URL+"/api/v1/downloads/view?cursor="+cursor)
		own, out := 0, 0
		for _, r := range v.Downloads {
			if r.ClientItem {
				out++
				if seen[r.Hash] {
					t.Fatalf("hash %s twice", r.Hash)
				}
				seen[r.Hash] = true
			} else {
				own++
			}
		}
		if own != 1 || out > OutsideCap || v.OutsideTotal != 450 {
			t.Fatalf("page %d: own %d outside %d total %d", pages, own, out, v.OutsideTotal)
		}
		pages++
		if v.NextCursor == "" {
			break
		}
		cursor = v.NextCursor
	}
	if pages != 3 || len(seen) != 450 {
		t.Fatalf("pages %d rows %d", pages, len(seen))
	}
	if v := view(t, x.srv.URL+"/api/v1/downloads/view?outside_limit=1000"); v.NextCursor != "" || len(v.Downloads) != 451 {
		t.Fatalf("one big page: %d rows next %q", len(v.Downloads), v.NextCursor)
	}
}

// ?asked=1 reads only downloads for titles with an approved ask, past the
// usual 500-row cap, so the Requests page can count failed asks exactly.
func TestDownloadsViewAsked(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "failed")
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', 'ana', 'member', 'x')`)
	get := func() int {
		var out viewRows
		_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view?asked=1", "", 200), &out)
		return len(out.Downloads)
	}
	if n := get(); n != 0 {
		t.Fatalf("no ask yet, got %d rows", n)
	}
	exec41(t, x.s, `INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
		SELECT 1, 't', '{}', 't', '{}', 'movie', '1', 'Salt Choir', 'asked', t.id, 2, 't' FROM titles t LIMIT 1`)
	if n := get(); n != 0 {
		t.Fatalf("ask not approved, got %d rows", n)
	}
	exec41(t, x.s, `UPDATE asks SET state = 'approved'`)
	if n := get(); n != 1 {
		t.Fatalf("approved ask, got %d rows", n)
	}
}
