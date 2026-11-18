package arrapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// moviePage is a page of the movie missing list.
type moviePage struct {
	Page, PageSize, TotalRecords int
	SortKey, SortDirection       string
	Records                      []struct {
		ID        int64
		Title     string
		TmdbID    int64
		HasFile   bool
		Monitored bool
	}
}

func readMoviePage(t *testing.T, r *pinRig, q string) moviePage {
	t.Helper()
	st, b := r.radarr("wanted/missing" + q)
	var p moviePage
	if st != http.StatusOK || json.Unmarshal(b, &p) != nil {
		t.Fatalf("wanted/missing%s: %d %s", q, st, b)
	}
	return p
}

func titles(p moviePage) string {
	var out []string
	for _, x := range p.Records {
		out = append(out, x.Title)
	}
	return strings.Join(out, ",")
}

// The Radarr side lists the movies that have no file in the main Version,
// kept up to date by default, whether or not they are out yet. Paging and
// sorting follow the old apps: no direction means descending, an unknown
// key falls back to the sort title, a page past the end is empty.
func TestRadarrWantedMissing(t *testing.T) {
	r := newPinRig(t)
	for i, n := range []string{"Bravo", "Alpha", "Delta", "Charlie"} {
		r.movie(n, fmt.Sprint(500+i), i)
	}
	unkept := r.movie("Unkept", "600", 10)
	r.exec("UPDATE titles SET keep_up_to_date = 0 WHERE id = ?", unkept)
	r.exec("UPDATE items SET keep_up_to_date = 0 WHERE title_id = ?", unkept)
	future := r.movie("Future", "601", 11) // not out yet: still listed, as Radarr does
	r.exec("UPDATE titles SET cinema_date = '2030-01-01T00:00:00Z', digital_date = NULL WHERE id = ?", future)
	gone := r.movie("Gone", "602", 12)
	r.exec("UPDATE titles SET state = 'removed' WHERE id = ?", gone)
	slotGone := r.movie("SlotGone", "603", 13)
	r.exec("UPDATE slots SET state = 'removed' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", slotGone)
	dropped := r.movie("Dropped", "604", 14)
	r.exec("UPDATE items SET dropped_at = ? WHERE title_id = ?", pinStamp, dropped)
	filed := r.movie("Filed", "605", 15)
	file := r.exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, version_id, relative_path, added_at, origin, placed_by)
		SELECT ?, '{}', ?, '{}', id, 'a.mkv', ?, 'imported', 'move' FROM versions WHERE title_id = ?`, pinStamp, pinStamp, pinStamp, filed)
	r.exec("UPDATE slots SET state = 'in_library', file_id = ? WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", file, filed)

	// Default page: ten a page, sort title, descending because no
	// direction was sent.
	p := readMoviePage(t, r, "")
	if p.Page != 1 || p.PageSize != 10 || p.TotalRecords != 5 || p.SortKey != "movieMetadata.sortTitle" || p.SortDirection != "descending" {
		t.Fatalf("default page: %+v", p)
	}
	if got := titles(p); got != "Future,Delta,Charlie,Bravo,Alpha" {
		t.Fatalf("default order: %s", got)
	}
	if x := p.Records[0]; x.HasFile || !x.Monitored || x.ID == 0 || x.TmdbID != 601 {
		t.Fatalf("record: %+v", x)
	}
	// Only "default" uses the endpoint's own ascending order.
	if p = readMoviePage(t, r, "?sortDirection=default"); p.SortDirection != "ascending" || titles(p) != "Alpha,Bravo,Charlie,Delta,Future" {
		t.Fatalf("default direction: %s %s", p.SortDirection, titles(p))
	}
	// Paging, in any case of the names.
	p = readMoviePage(t, r, "?PAGE=2&pagesize=2&sortDirection=ascending")
	if p.Page != 2 || p.PageSize != 2 || p.TotalRecords != 5 || titles(p) != "Charlie,Delta" {
		t.Fatalf("page 2: %+v %s", p, titles(p))
	}
	// A page past the end is empty and keeps the asked number.
	if p = readMoviePage(t, r, "?page=99"); p.Page != 99 || len(p.Records) != 0 || p.TotalRecords != 5 {
		t.Fatalf("past the end: %+v", p)
	}
	// Not kept up to date gives the other movie.
	if p = readMoviePage(t, r, "?monitored=false"); p.TotalRecords != 1 || titles(p) != "Unkept" || p.Records[0].Monitored {
		t.Fatalf("monitored=false: %+v", p)
	}
	// An unknown key falls back; every real key answers.
	if p = readMoviePage(t, r, "?sortKey=nope"); p.SortKey != "movieMetadata.sortTitle" {
		t.Fatalf("unknown key: %s", p.SortKey)
	}
	for _, k := range []string{"movieMetadata.digitalRelease", "movieMetadata.inCinemas", "movieMetadata.physicalRelease", "movieMetadata.year", "movies.lastSearchTime"} {
		if p = readMoviePage(t, r, "?sortKey="+k); p.SortKey != k || p.TotalRecords != 5 || len(p.Records) != 5 {
			t.Fatalf("sort key %s: %+v", k, p)
		}
	}
	// Sorted by cinema date, the movie that comes out last leads.
	if p = readMoviePage(t, r, "?sortKey=movieMetadata.inCinemas"); p.Records[0].Title != "Future" {
		t.Fatalf("by cinema date: %s", titles(p))
	}
	// A bad number is the default, not an error.
	if p = readMoviePage(t, r, "?page=x&pageSize=-3"); p.Page != 1 || p.PageSize != 10 {
		t.Fatalf("bad numbers: %+v", p)
	}
}

// Each record is a movie as the movie list gives it.
func TestRadarrWantedMissingRecordIsAMovie(t *testing.T) {
	r := newPinRig(t)
	id := r.movie("Alpha", "500", 1)
	_, b := r.radarr("wanted/missing")
	var page struct{ Records []map[string]any }
	if err := json.Unmarshal(b, &page); err != nil || len(page.Records) != 1 {
		t.Fatalf("%v %s", err, b)
	}
	_, one := r.radarr(fmt.Sprintf("movie/%d", id))
	var want map[string]any
	if err := json.Unmarshal(one, &want); err != nil {
		t.Fatal(err)
	}
	got := page.Records[0]
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("record lacks %q that the movie has", k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("record has %d fields, the movie %d", len(got), len(want))
	}
}

type calMovie struct {
	ID        int64
	Title     string
	Monitored bool
}

func readCalendar(t *testing.T, r *pinRig, q string) []calMovie {
	t.Helper()
	st, b := r.radarr("calendar" + q)
	var out []calMovie
	if st != http.StatusOK || json.Unmarshal(b, &out) != nil {
		t.Fatalf("calendar%s: %d %s", q, st, b)
	}
	return out
}

func calTitles(ms []calMovie) string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Title)
	}
	return strings.Join(out, ",")
}

// The Radarr calendar lists movies by cinema, digital and physical release
// date. The window includes both ends, defaults to today and two days on,
// leaves out movies that are not kept unless unmonitored=true, and lists a
// movie once even when more than one of its dates is inside.
func TestRadarrCalendar(t *testing.T) {
	r := newPinRig(t)
	set := func(name string, i int, cinema, digital, physical string) int64 {
		id := r.movie(name, fmt.Sprint(700+i), i)
		r.exec("UPDATE titles SET cinema_date = NULLIF(?, ''), digital_date = NULLIF(?, ''), physical_date = NULLIF(?, '') WHERE id = ?", cinema, digital, physical, id)
		return id
	}
	set("Today", 1, pinDay(0), "", "")
	set("Tomorrow", 2, "", pinDay(1), "")
	set("Both", 3, pinDay(0), pinDay(1), pinDay(2)) // three dates inside the window, one entry
	set("Far", 4, pinDay(50), "", "")
	set("Past", 5, pinDay(-5), "", "")
	unkept := set("Unkept", 6, pinDay(1), "", "")
	r.exec("UPDATE titles SET keep_up_to_date = 0 WHERE id = ?", unkept)
	gone := set("Gone", 7, pinDay(1), "", "")
	r.exec("UPDATE titles SET state = 'removed' WHERE id = ?", gone)
	set("Dateless", 8, "", "", "")

	// Today to two days on, by cinema date, then digital, then physical, a
	// missing date last, each movie once.
	got := readCalendar(t, r, "")
	if calTitles(got) != "Both,Today,Tomorrow" {
		t.Fatalf("default window: %s", calTitles(got))
	}
	for _, m := range got {
		if m.ID == 0 || !m.Monitored {
			t.Fatalf("movie %+v", m)
		}
	}
	// With unmonitored the movie that is not kept shows too.
	if got = readCalendar(t, r, "?unmonitored=true"); calTitles(got) != "Both,Today,Unkept,Tomorrow" {
		t.Fatalf("unmonitored: %s", calTitles(got))
	}
	// An explicit window includes its end: the end day itself counts.
	day := pinClock.Format("2006-01-02")
	if got = readCalendar(t, r, "?start=2026-10-13T00:00:00Z&end=2026-10-14T12:00:00Z"); calTitles(got) != "Both,Today,Tomorrow" {
		t.Fatalf("explicit window: %s", calTitles(got))
	}
	if got = readCalendar(t, r, "?start="+day+"T12:00:00Z&end="+day+"T12:00:00Z"); calTitles(got) != "Both,Today" {
		t.Fatalf("one instant: %s", calTitles(got))
	}
	// Plain dates are read as midnight UTC.
	if got = readCalendar(t, r, "?start=2026-10-08&end=2026-10-08"); len(got) != 0 {
		t.Fatalf("a day before the past movie's time: %s", calTitles(got))
	}
	if got = readCalendar(t, r, "?start=2026-10-08T12:00:00Z&end=2026-10-08T12:00:00Z"); calTitles(got) != "Past" {
		t.Fatalf("the past movie: %s", calTitles(got))
	}
	// A window that ends before it starts is empty, not an error.
	if got = readCalendar(t, r, "?start=2026-10-20&end=2026-10-14"); got == nil || len(got) != 0 {
		t.Fatalf("backwards: %#v", got)
	}
	// Nothing at all answers a list, never null.
	st, b := r.radarr("calendar?start=2030-01-01&end=2030-01-02")
	if st != http.StatusOK || strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("empty: %d %s", st, b)
	}
}

// both runs the same request on both sides.
func (r *pinRig) both(path string) map[string][]byte {
	r.t.Helper()
	out := map[string][]byte{}
	for _, side := range []string{"sonarr", "radarr"} {
		var h http.Handler = Sonarr(Deps{Store: r.st, Now: func() time.Time { return pinClock }})
		if side == "radarr" {
			h = Radarr(Deps{Store: r.st, Now: func() time.Time { return pinClock }})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/"+side+"/api/v3/"+path, nil))
		if rec.Code != http.StatusOK {
			r.t.Fatalf("%s %s: %d %s", side, path, rec.Code, rec.Body)
		}
		out[side] = rec.Body.Bytes()
	}
	return out
}

// /health lists what is wrong, both sides alike, in the old apps' shape: a
// healthy app gives an empty list, and a check that passes is left out.
func TestHealthBothSides(t *testing.T) {
	r := newPinRig(t)
	for side, b := range r.both("health") {
		if strings.TrimSpace(string(b)) != "[]" {
			t.Fatalf("%s, nothing wrong: %s", side, b)
		}
	}
	add := func(key, kind, subject, state, msg string) {
		r.exec(`INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)`,
			key, kind, subject, state, msg, pinStamp, pinStamp, state)
	}
	add("downloader:1", "downloader", "qBit", "warning", "Downloader qBit does not answer")
	add("library_folder:2", "library_folder", "Movies", "error", "Library folder Movies cannot be reached (is the disk or share mounted?)")
	add("backups", "backups", "", "ok", "")
	for side, b := range r.both("health") {
		var got []healthItem
		if err := json.Unmarshal(b, &got); err != nil || len(got) != 2 {
			t.Fatalf("%s: %v %s", side, err, b)
		}
		// The worst first, as the Health page lists them.
		if got[0].Type != "error" || got[1].Type != "warning" || !strings.Contains(got[0].Message, "Movies cannot be reached") || got[0].Source == "" || got[1].Source == "" {
			t.Fatalf("%s: %+v", side, got)
		}
	}
}

// The shapes of the three answers are pinned, so a change shows up as a
// failing test. They match what the real Radarr answered in the lab: the
// paging envelope with movies as records, a list of movies, and a list of
// problems with a source, a type, a message and a link.
func TestPinRadarrListShapes(t *testing.T) {
	r := newPinRig(t)
	r.movie("Alpha", "500", 1)
	r.exec(`INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed) VALUES ('downloader:1', 'downloader', 'qBit', 'warning', 'Downloader qBit does not answer', ?, ?, 1, 'warning')`, pinStamp, pinStamp)
	_, b := r.radarr("wanted/missing")
	pinGoldenShape(t, "radarr_missing", b)
	_, b = r.radarr("calendar?start=2020-01-01&end=2020-01-02")
	pinGoldenShape(t, "radarr_calendar", b)
	_, b = r.radarr("health")
	pinGoldenShape(t, "health", b)
}

// A big Library still pages the missing movies and lists the calendar
// quickly: 10,000 movies, a few with a file, a few not kept.
func TestPinRadarrListsOverALargeLibrary(t *testing.T) {
	r := newPinRig(t)
	const n = 10000
	r.exec(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES (?, '{}', ?, '{}', ?, 'movie', 'Movies', 'ok', '[]')`,
		pinStamp, pinStamp, t.TempDir())
	r.exec(`WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < ?)
		INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state, cinema_date)
		SELECT ?, '{}', ?, '{}', 'movie', 'Bulk ' || i, printf('bulk %05d', i), 1950 + i % 70, '{"tmdb":"' || (90000 + i) || '"}', 'tmdb', 'import', 'active',
			strftime('%Y-%m-%dT00:00:00Z', ?, '+' || (i % 400) || ' days') FROM seq`, n, pinStamp, pinStamp, pinStamp)
	r.exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
		SELECT ?, '{}', ?, '{}', id, 'Main', ?, (SELECT id FROM library_folders WHERE kind = 'movie' LIMIT 1), name, 1 FROM titles WHERE name LIKE 'Bulk %'`, pinStamp, pinStamp, r.rules)
	r.exec(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind) SELECT ?, '{}', ?, '{}', id, 'movie' FROM titles WHERE name LIKE 'Bulk %'`, pinStamp, pinStamp)
	r.exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state)
		SELECT ?, '{}', ?, '{}', i.id, v.id, 'wanted' FROM items i JOIN versions v ON v.title_id = i.title_id`, pinStamp, pinStamp)
	r.exec(`UPDATE titles SET keep_up_to_date = 0 WHERE name LIKE 'Bulk %' AND year = 1950`) // about 143 not kept
	var unkept int
	if err := r.st.Read().QueryRow(`SELECT count(*) FROM titles WHERE name LIKE 'Bulk %' AND keep_up_to_date = 0`).Scan(&unkept); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	p := readMoviePage(t, r, "?pageSize=50&page=3&sortKey=movieMetadata.year&sortDirection=ascending")
	if p.TotalRecords != n-unkept || len(p.Records) != 50 || p.Page != 3 {
		t.Fatalf("page: total %d (want %d) n %d", p.TotalRecords, n-unkept, len(p.Records))
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("the missing page took %s", d)
	}
	if p = readMoviePage(t, r, "?monitored=false&pageSize=1000"); p.TotalRecords != unkept || len(p.Records) != min(unkept, 1000) {
		t.Fatalf("not kept: %d", p.TotalRecords)
	}
	// The biggest page a client may ask for.
	start = time.Now()
	if p = readMoviePage(t, r, "?pageSize=10000"); len(p.Records) != n-unkept {
		t.Fatalf("whole list: %d", len(p.Records))
	}
	t.Logf("the whole missing list of %d movies took %s", len(p.Records), time.Since(start))
	start = time.Now()
	got := readCalendar(t, r, "?start=2026-10-13&end=2026-10-20")
	if len(got) == 0 {
		t.Fatal("the calendar found nothing")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("the calendar took %s", d)
	}
}

// The Radarr side answers the download list unpaged and its counts, which
// the Homepage widget asks for each time it draws; both sides answer the
// counts.
func TestRadarrQueueDetailsAndStatus(t *testing.T) {
	r := newPinRig(t)
	dl := r.exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES (?, '{}', ?, '{}', 'qb', 'qbittorrent', 'torrent', 'http://x')`, pinStamp, pinStamp)
	var movies []int64
	for i, state := range []string{"downloading", "stalled", "queued"} {
		title := r.movie(fmt.Sprintf("Queued %d", i), fmt.Sprint(700+i), i)
		movies = append(movies, title)
		var slot int64
		_ = r.st.Read().QueryRow("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?", title).Scan(&slot)
		r.exec(`INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, progress, state, started_by)
			VALUES (?, '{}', ?, '{}', ?, ?, ?, 0.5, ?, '{}')`, pinStamp, pinStamp, dl, fmt.Sprintf("[%d]", slot), fmt.Sprintf("m%d", i), state)
	}
	st, b := r.radarr("queue/details")
	var rows []struct{ MovieID int64 }
	if st != http.StatusOK || json.Unmarshal(b, &rows) != nil || len(rows) != 3 {
		t.Fatalf("details: %d %s", st, b)
	}
	if _, b = r.radarr(fmt.Sprintf("queue/details?movieId=%d", movies[1])); !strings.Contains(string(b), fmt.Sprint(movies[1])) || strings.Count(string(b), `"movieId"`) != 1 {
		t.Fatalf("movieId filter: %s", b)
	}
	if _, b = r.radarr("queue/details?movieId=999999"); strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("no such movie: %s", b)
	}
	for side, b := range r.both("queue/status") {
		var s map[string]int
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("%s: %v %s", side, err, b)
		}
		if side == "radarr" && (s["totalCount"] != 3 || s["count"] != 3 || s["warnings"] != 1 || s["errors"] != 0) {
			t.Fatalf("%s: %s", side, b)
		}
		for _, k := range []string{"totalCount", "count", "unknownCount", "errors", "warnings", "unknownErrors", "unknownWarnings"} {
			if _, ok := s[k]; !ok {
				t.Fatalf("%s lacks %s: %s", side, k, b)
			}
		}
	}
}

// /health counts a check the way Home does: by its confirmed state, so a
// failure seen once is left out and one still held while it recovers stays.
func TestHealthFollowsConfirmed(t *testing.T) {
	r := newPinRig(t)
	r.exec(`INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed) VALUES ('downloader:1', 'downloader', 'qBit', 'error', 'Downloader qBit timed out', ?, ?, 1, 'ok')`, pinStamp, pinStamp)
	r.exec(`INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed) VALUES ('downloader:2', 'downloader', 'SAB', 'ok', 'Downloader SAB does not answer', ?, ?, 5, 'warning')`, pinStamp, pinStamp)
	for side, b := range r.both("health") {
		var got []healthItem
		if err := json.Unmarshal(b, &got); err != nil || len(got) != 1 || got[0].Type != "warning" || !strings.Contains(got[0].Message, "SAB") {
			t.Fatalf("%s: %v %s", side, err, b)
		}
	}
}

// A calendar window as wide as the dates allow still answers each movie
// once and quickly.
func TestCalendarWholeRangeOnceEach(t *testing.T) {
	r := newPinRig(t)
	for i := 0; i < 300; i++ {
		r.movie(fmt.Sprintf("Wide %03d", i), fmt.Sprint(8000+i), i)
	}
	start := time.Now()
	got := readCalendar(t, r, "?start=0001-01-01&end=9999-12-31")
	if len(got) != 300 {
		t.Fatalf("%d movies", len(got))
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %s", d)
	}
}
