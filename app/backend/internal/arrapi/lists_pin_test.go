package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

var pinClock = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

const pinStamp = "2026-10-13T12:00:00Z"

type pinRig struct {
	t      *testing.T
	st     *database.Store
	son    http.Handler
	folder int64
	rules  int64
}

func newPinRig(t *testing.T) *pinRig {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := &pinRig{t: t, st: st}
	err = st.Write(context.Background(), func(tx *sql.Tx) error {
		id, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "show", Label: "Shows", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		r.folder = id
		if err != nil {
			return err
		}
		return tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&r.rules)
	})
	if err != nil {
		t.Fatal(err)
	}
	r.son = Sonarr(Deps{Store: st, Now: func() time.Time { return pinClock }})
	return r
}

func (r *pinRig) exec(q string, args ...any) int64 {
	r.t.Helper()
	var id int64
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec(q, args...)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return id
}

// show adds a show, kept or not, with one season of n episodes. air(k)
// gives the air date of episode k, and "" stores NULL.
func (r *pinRig) show(name, tmdb string, keep bool, n int, air func(k int) string) (title int64, items []int64) {
	r.t.Helper()
	k := 0
	if keep {
		k = 1
	}
	title = r.exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state, keep_up_to_date)
		VALUES (?, '{}', ?, '{}', 'show', ?, ?, 2020, ?, 'tmdb', 'import', 'active', ?)`, pinStamp, pinStamp, name, strings.ToLower(name), fmt.Sprintf(`{"tmdb":"%s"}`, tmdb), k)
	part := r.exec(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, number, name, keep_up_to_date) VALUES (?, '{}', ?, '{}', ?, 'season', 1, 'Season 1', 1)`,
		pinStamp, pinStamp, title)
	ver := r.exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
		VALUES (?, '{}', ?, '{}', ?, 'Main', ?, ?, ?, 1)`, pinStamp, pinStamp, title, r.rules, r.folder, name)
	for e := 1; e <= n; e++ {
		var a, day any
		timed := 0
		if d := air(e); d != "" {
			a, day, timed = d, d[:10], 1
		}
		it := r.exec(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, part_id, kind, numbers, air_date, air_day, air_timed)
			VALUES (?, '{}', ?, '{}', ?, ?, 'episode', ?, ?, ?, ?)`, pinStamp, pinStamp, title, part, fmt.Sprintf(`{"season":1,"episode":%d}`, e), a, day, timed)
		r.exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, '{}', ?, '{}', ?, ?, 'wanted')`, pinStamp, pinStamp, it, ver)
		items = append(items, it)
	}
	return title, items
}

func (r *pinRig) get(path string) (int, []byte) {
	r.t.Helper()
	rec := httptest.NewRecorder()
	r.son.ServeHTTP(rec, httptest.NewRequest("GET", "/sonarr/api/v3/"+path, nil))
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, b
}

func (r *pinRig) send(method, path, body string) (int, []byte) {
	r.t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/sonarr/api/v3/"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.son.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, b
}

func pinDay(n int) string { return database.FormatTime(pinClock.AddDate(0, 0, n)) }

type pinPage struct {
	Page, PageSize, TotalRecords int
	SortKey, SortDirection       string
	Records                      []struct {
		ID       int64
		SeriesID int64
		Title    string
	}
}

func decodePage(t *testing.T, b []byte) pinPage {
	t.Helper()
	var p pinPage
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return p
}

// The missing list joins items, titles, versions and slots over a big
// library. It gives aired episodes with no file in the main Version, kept
// ones by default.
func TestPinWantedMissingOverALargeLibrary(t *testing.T) {
	r := newPinRig(t)
	// 120 kept shows, 3 episodes each, aired 1..3 days apart in the past.
	var firstItems []int64
	for i := 0; i < 120; i++ {
		_, items := r.show(fmt.Sprintf("Show %03d", i), fmt.Sprint(1000+i), true, 3, func(k int) string { return pinDay(-(i%30 + k)) })
		firstItems = append(firstItems, items...)
	}
	// The list must skip an unmonitored show, a future episode, an episode
	// with a NULL air date, a dropped one, a removed slot and a file.
	r.show("Unkept", "2001", false, 2, func(k int) string { return pinDay(-k) })
	r.show("Future", "2002", true, 1, func(int) string { return pinDay(5) })
	r.show("No Date", "2003", true, 1, func(int) string { return "" })
	_, di := r.show("Dropped", "2004", true, 1, func(int) string { return pinDay(-2) })
	r.exec("UPDATE items SET dropped_at = ? WHERE id = ?", pinStamp, di[0])
	_, ri := r.show("Slot Removed", "2005", true, 1, func(int) string { return pinDay(-2) })
	r.exec("UPDATE slots SET state = 'removed' WHERE item_id = ?", ri[0])
	_, fi := r.show("Has File", "2006", true, 1, func(int) string { return pinDay(-2) })
	file := r.exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, version_id, relative_path, added_at, origin, placed_by)
		SELECT ?, '{}', ?, '{}', version_id, 'a.mkv', ?, 'imported', 'move' FROM slots WHERE item_id = ?`, pinStamp, pinStamp, pinStamp, fi[0])
	r.exec("UPDATE slots SET state = 'in_library', file_id = ? WHERE item_id = ?", file, fi[0])

	st, b := r.get("wanted/missing")
	if st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	p := decodePage(t, b)
	if p.TotalRecords != 360 || p.PageSize != 10 || p.Page != 1 || len(p.Records) != 10 || p.SortKey != "episodes.airDateUtc" || p.SortDirection != "descending" {
		t.Fatalf("default page: total %d size %d page %d n %d %s %s", p.TotalRecords, p.PageSize, p.Page, len(p.Records), p.SortKey, p.SortDirection)
	}
	// Descending: the most recently aired first.
	st, b = r.get("wanted/missing?pageSize=400")
	p = decodePage(t, b)
	if st != 200 || len(p.Records) != 360 {
		t.Fatalf("all: %d %d", st, len(p.Records))
	}
	for _, x := range p.Records {
		if strings.HasPrefix(x.Title, "") && x.SeriesID == 0 {
			t.Fatalf("record without series id: %+v", x)
		}
	}
	// Ascending by air date, page 2 of size 100.
	st, b = r.get("wanted/missing?sortKey=episodes.airDateUtc&sortDirection=ascending&pageSize=100&page=2")
	p = decodePage(t, b)
	if st != 200 || p.Page != 2 || len(p.Records) != 100 || p.SortDirection != "ascending" {
		t.Fatalf("asc page 2: %d %d %s", st, len(p.Records), p.SortDirection)
	}
	// A page past the end is empty and keeps the asked page number.
	st, b = r.get("wanted/missing?pageSize=100&page=9")
	p = decodePage(t, b)
	if st != 200 || p.Page != 9 || len(p.Records) != 0 || p.TotalRecords != 360 {
		t.Fatalf("past end: %d %+v", st, p)
	}
	// Unmonitored gives only the unkept show's two episodes.
	st, b = r.get("wanted/missing?monitored=false")
	p = decodePage(t, b)
	if st != 200 || p.TotalRecords != 2 {
		t.Fatalf("unmonitored: %d total %d", st, p.TotalRecords)
	}
	// An unknown sort key falls back to the air date.
	if _, b = r.get("wanted/missing?sortKey=nope"); decodePage(t, b).SortKey != "episodes.airDateUtc" {
		t.Fatalf("unknown sort key: %s", b)
	}
	// Sorted by show title ascending, the first record is Show 000's.
	_, b = r.get("wanted/missing?sortKey=series.sortTitle&sortDirection=ascending&pageSize=3&includeSeries=true")
	var withSeries struct {
		Records []struct {
			Series struct{ Title string }
		}
	}
	if err := json.Unmarshal(b, &withSeries); err != nil || len(withSeries.Records) != 3 || withSeries.Records[0].Series.Title != "Show 000" {
		t.Fatalf("by title: %v %s", err, b)
	}
}

// The calendar reads items by air time and then keeps the monitored ones.
func TestPinCalendarWindowAndMonitoring(t *testing.T) {
	r := newPinRig(t)
	for i := 0; i < 100; i++ { // a crowd far from the window
		r.show(fmt.Sprintf("Far %03d", i), fmt.Sprint(3000+i), true, 2, func(k int) string { return pinDay(100 + k) })
	}
	_, a := r.show("Soon A", "4001", true, 2, func(k int) string { return pinDay(k - 1) }) // day 0 and day 1
	_, u := r.show("Soon Unkept", "4002", false, 1, func(int) string { return pinDay(1) }) // unmonitored
	_, o := r.show("Soon Dropped", "4003", true, 1, func(int) string { return pinDay(1) }) // dropped
	r.exec("UPDATE items SET dropped_at = ? WHERE id = ?", pinStamp, o[0])
	_, _ = u, a
	// An episode with a NULL air_date is placed by its air day.
	_, nd := r.show("Day Only", "4004", true, 1, func(int) string { return "" })
	r.exec("UPDATE items SET air_day = '2026-10-14' WHERE id = ?", nd[0])
	type ep struct {
		ID           int64
		SeriesID     int64
		AirDateUtc   string
		Series       *struct{ Title string }
		SeasonNumber int
	}
	read := func(q string) []ep {
		st, b := r.get("calendar?" + q)
		var out []ep
		if st != 200 || json.Unmarshal(b, &out) != nil {
			t.Fatalf("%s: %d %s", q, st, b)
		}
		return out
	}
	// Default window: today to two days on.
	got := read("")
	if len(got) != 3 || got[0].AirDateUtc != "2026-10-13T12:00:00Z" || got[1].AirDateUtc != "" || got[2].AirDateUtc != "2026-10-14T12:00:00Z" {
		t.Fatalf("default window: %d episodes %+v", len(got), got)
	}
	// With unmonitored the unkept show appears too. The dropped one never
	// does.
	if got = read("unmonitored=true"); len(got) != 4 {
		t.Fatalf("unmonitored: %d", len(got))
	}
	// An explicit window and the show on each episode.
	got = read("start=2026-10-13&end=2026-10-14&includeSeries=true")
	if len(got) != 1 || got[0].Series == nil || got[0].Series.Title != "Soon A" {
		t.Fatalf("one day: %+v", got)
	}
	// An end before the start gives an empty list, not an error.
	if got = read("start=2026-10-20&end=2026-10-14"); len(got) != 0 {
		t.Fatalf("backwards: %+v", got)
	}
}

// The queue joins downloads with releases, sources, downloaders and
// slots. The first three are LEFT JOINs, so a download with none of them
// still shows.
func TestPinQueueDetailsWithMissingPartners(t *testing.T) {
	r := newPinRig(t)
	dl := r.exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES (?, '{}', ?, '{}', 'qb', 'qbittorrent', 'torrent', 'http://x')`, pinStamp, pinStamp)
	var all []int64
	for i := 0; i < 60; i++ { // a crowd of other shows with finished, unrelated downloads
		_, items := r.show(fmt.Sprintf("Crowd %02d", i), fmt.Sprint(5000+i), true, 1, func(int) string { return pinDay(-3) })
		all = append(all, items[0])
	}
	slotOf := func(item int64) int64 {
		var id int64
		_ = r.st.Read().QueryRow("SELECT id FROM slots WHERE item_id = ?", item).Scan(&id)
		return id
	}
	add := func(ext, state string, slots ...int64) int64 {
		js, _ := json.Marshal(slots)
		return r.exec(`INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, progress, state, started_by)
			VALUES (?, '{}', ?, '{}', ?, ?, ?, 0.5, ?, '{}')`, pinStamp, pinStamp, dl, string(js), ext, state)
	}
	a := add("h1", "downloading", slotOf(all[0]))       // no release, no source
	add("h2", "queued", slotOf(all[1]), slotOf(all[2])) // two slots, no release
	add("h3", "finished")                               // finished and in no slot: not in the queue
	f := add("h4", "finished", slotOf(all[3]))          // finished but a slot still points at it
	r.exec("UPDATE slots SET active_download_id = ? WHERE id = ?", f, slotOf(all[3]))
	add("h5", "failed", slotOf(all[4]))                                                                        // failed: not in the queue
	add("h6", "downloading")                                                                                   // no slots: leaves no row
	r.exec("UPDATE titles SET state = 'removed' WHERE id = (SELECT title_id FROM items WHERE id = ?)", all[2]) // its row drops out
	_ = a

	st, b := r.get("queue/details?includeEpisode=true")
	var rows []struct {
		ID, SeriesID, EpisodeID int64
		DownloadId              string
		Protocol, Title         string
		Episode                 *struct{ SeasonNumber, EpisodeNumber int }
	}
	if st != 200 || json.Unmarshal(b, &rows) != nil {
		t.Fatalf("%d %s", st, b)
	}
	// h1 gives 1 row, h2 gives 1 row since one slot is on a removed title, h4 gives 1 row.
	if len(rows) != 3 {
		t.Fatalf("%d rows: %s", len(rows), b)
	}
	for _, x := range rows {
		if x.Episode == nil || x.Episode.SeasonNumber != 1 || x.Episode.EpisodeNumber != 1 || x.SeriesID == 0 {
			t.Fatalf("row without its episode: %+v", x)
		}
	}
	// Narrow to one show, then to nothing.
	_, b = r.get(fmt.Sprintf("queue/details?seriesId=%d", rows[0].SeriesID))
	var one []struct{ SeriesID int64 }
	_ = json.Unmarshal(b, &one)
	if len(one) != 1 || one[0].SeriesID != rows[0].SeriesID {
		t.Fatalf("seriesId filter: %s", b)
	}
	if _, b = r.get("queue/details?seriesId=999999"); strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("no such show: %s", b)
	}
}

// Tags keep one row per label, listed by id. A second add of the same
// label returns the first. Renaming a missing id is a 404.
func TestPinTagsStore(t *testing.T) {
	r := newPinRig(t)
	var ids []int64
	for _, l := range []string{"anime", "kids", " anime "} {
		st, b := r.send("POST", "tag", fmt.Sprintf(`{"label":%q}`, l))
		var x struct {
			ID    int64
			Label string
		}
		_ = json.Unmarshal(b, &x)
		if st != 201 || x.Label != strings.TrimSpace(l) {
			t.Fatalf("add %q: %d %s", l, st, b)
		}
		ids = append(ids, x.ID)
	}
	if ids[0] != ids[2] || ids[0] == ids[1] {
		t.Fatalf("ids %v", ids)
	}
	if st, _ := r.send("POST", "tag", `{"label":""}`); st != 400 {
		t.Fatalf("empty label: %d", st)
	}
	if st, _ := r.send("PUT", fmt.Sprintf("tag/%d", ids[1]), `{"label":"anime"}`); st != 400 {
		t.Fatalf("rename to a taken label: %d", st)
	}
	if st, b := r.send("PUT", fmt.Sprintf("tag/%d", ids[1]), `{"label":"family"}`); st != 202 || !strings.Contains(string(b), "family") {
		t.Fatalf("rename: %d %s", st, b)
	}
	if st, _ := r.send("PUT", "tag/999", `{"label":"x"}`); st != 404 {
		t.Fatalf("rename missing: %d", st)
	}
	_, b := r.get("tag")
	var list []struct {
		ID    int64
		Label string
	}
	_ = json.Unmarshal(b, &list)
	if len(list) != 2 || list[0].Label != "anime" || list[1].Label != "family" {
		t.Fatalf("list %s", b)
	}
}
