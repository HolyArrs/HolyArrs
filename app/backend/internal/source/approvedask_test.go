package source

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// askFor adds an ask for the title in the given state, decided at decided.
func askFor(t *testing.T, f fixture, titleID int64, state, decided string) {
	t.Helper()
	exec(t, f, `INSERT OR IGNORE INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'ana', 'member', 'x')`)
	exec(t, f, `INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at, decided_at)
		SELECT 't', '{}', 't', '{}', t.kind, CAST(t.id AS TEXT), t.name, ?, t.id, 1, 't', ? FROM titles t WHERE t.id = ?`, state, decided, titleID)
}

func wantedJobs(t *testing.T, f fixture, titleID int64) int {
	t.Helper()
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND dedupe_key = ? AND json_extract(params, '$.title_id') = ?",
		KindSearchMissing, "search_wanted:"+strconv.FormatInt(titleID, 10), titleID).Scan(&n)
	return n
}

func episodeSearches(t *testing.T, f showFix) int {
	t.Helper()
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind IN (?, ?) AND (json_extract(params, '$.version_id') = ? OR json_extract(subject, '$.id') = ?)",
		KindSearchSeason, KindSearchSlot, f.verID, f.slots["S02E01"]).Scan(&n)
	return n
}

// An approved request behind a Library that was never searched: the
// scheduled search takes the asked title first, through the same one-title
// search approval starts, and spends its cap on the rest. On 18729, asks
// 34, 10 and 11 sat behind 7,000 never-searched Slots at 20 a day.
func TestMissingSearchApprovedAskFirst(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	setSetting(t, f.fixture, SettingMissingCount, "1")
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	// The movie was searched before, so without the exclusion the show's
	// never-searched episodes would take the one place.
	exec(t, f.fixture, "UPDATE items SET last_searched_at = '2020-01-01T00:00:00Z' WHERE id = (SELECT item_id FROM slots WHERE id = ?)", f.slotID)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["asks"].(float64); n != 1 {
		t.Fatalf("asks caught up: %+v", j.Result)
	}
	if wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatal("the asked show did not get its one-title search")
	}
	// The cap of one went to the movie, not to one of the show's episodes.
	var movie int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') = ?", KindSearchSlot, f.slotID).Scan(&movie)
	// (The show's own episodes are searched by its one-title Job, which the
	// test queue may run at once; the capped list itself queued only the movie.)
	n, _ := j.Result["slots"].(float64)
	queued, _ := j.Result["queued"].(float64)
	if n != 1 || queued != 1 || movie != 1 {
		t.Fatalf("rest of the cap: %+v, movie searches %d", j.Result, movie)
	}
}

// Once per approval: a one-title search already queued since the approval
// (here its Sources all failed, so nothing was stamped) is not queued
// again by the catch-up; nor is a title whose approval scan and search are
// still to run.
func TestMissingSearchApprovedAskNotRepeated(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	sr := &Searcher{Store: f.s}
	// Each case starts with no Jobs and nothing searched (a catch-up in an
	// earlier case may have run its search in the test queue).
	fresh := func() {
		exec(t, f.fixture, "DELETE FROM jobs")
		exec(t, f.fixture, "UPDATE items SET last_searched_at = NULL")
	}
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-11-01T04:00:00Z', '{}', 't', '{}', ?, '{"type":"title","id":1}', '{}', 'normal', 'done', 1, 5, 't', '{}', ?)`,
		KindSearchMissing, "search_wanted:"+strconv.FormatInt(f.titleID, 10))
	j := runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["asks"].(float64); n != 0 {
		t.Fatalf("caught up twice: %+v", j.Result)
	}
	// An older one-title search, from before the approval, does not count.
	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-10-01T04:00:00Z', '{}', 't', '{}', ?, '{"type":"title","id":1}', '{}', 'normal', 'done', 1, 5, 't', '{}', ?)`,
		KindSearchMissing, "search_wanted:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 {
		t.Fatalf("a search from before the approval blocked the catch-up: %+v", j.Result)
	}
	// A one-title search still queued from before the approval (a person's
	// or an older approval's) is not queued twice.
	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-10-01T04:00:00Z', '{}', 't', '{}', ?, '{"type":"title","id":1}', '{}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}', ?)`,
		KindSearchMissing, "search_wanted:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 {
		t.Fatalf("caught up while its one-title search was queued: %+v", j.Result)
	}
	// Approval's own scan, still queued, searches when it is done.
	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-11-04T04:00:00Z', '{}', 't', '{}', 'scan_title', ?, '{"then":["search_wanted"]}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}', ?)`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`, "scan_then_search:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("caught up while approval's scan was queued: %+v", j.Result)
	}
	// An artist's approval refreshes first (no scan key yet), then searches.
	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by)
		VALUES ('2026-11-04T04:00:00Z', '{}', 't', '{}', 'refresh_title', ?, '{"then":["scan_title","search_wanted"]}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}')`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`)
	j = runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("caught up while approval's refresh was queued: %+v", j.Result)
	}
	// After that refresh, the chain's scan (its own key) is queued.
	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-10-01T04:00:00Z', '{}', 't', '{}', 'scan_title', ?, '{"then":["search_wanted"]}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}', ?)`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`, "scan_title:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("caught up while the chain's scan was queued: %+v", j.Result)
	}
	// A plain refresh queued (no search after it) does not hold it back.
	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-11-04T04:00:00Z', '{}', 't', '{}', 'refresh_title', ?, '{}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}', ?)`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`, "refresh_title:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 {
		t.Fatalf("a plain refresh held the catch-up back: %+v", j.Result)
	}
	// Approval's scan failed, so it never queued its search: caught up.
	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-11-04T04:00:00Z', '{}', 't', '{}', 'scan_title', ?, '{"then":["search_wanted"]}', 'normal', 'failed', 5, 5, 't', '{}', ?)`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`, "scan_then_search:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatalf("a failed approval scan blocked the catch-up: %+v", j.Result)
	}
}

// Oldest approval first, and no more titles than missing_search_count.
func TestMissingSearchApprovedAsksOldestFirstAndCapped(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	var movieTitle int64
	_ = f.s.Read().QueryRow("SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", f.slotID).Scan(&movieTitle)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	askFor(t, f.fixture, movieTitle, "approved", "2026-10-30T22:21:16Z")
	setSetting(t, f.fixture, SettingMissingCount, "1")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, movieTitle) != 1 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("older approval first, one title: %+v movie %d show %d", j.Result, wantedJobs(t, f.fixture, movieTitle), wantedJobs(t, f.fixture, f.titleID))
	}
}

// Music like shows: an approved artist is caught up with the nightly music
// switch off, unless "Search when approved" is off too.
func TestMissingSearchApprovedArtistMusicSwitch(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return nil }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', ?, 'false', '2026-01-01', '{}', '2026-01-01', '{}')`, SettingMissingMusic)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T01:33:30Z")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatalf("approved artist with music off: %+v", j.Result)
	}
	exec(t, f.fixture, "DELETE FROM jobs")
	exec(t, f.fixture, "UPDATE items SET last_searched_at = NULL")
	exec(t, f.fixture, `INSERT INTO settings (created_at, created_by, updated_at, updated_by, grp, key, value) VALUES ('t', '{}', 't', '{}', 'this_app', 'ask_search_on_approve', 'false')`)
	j = runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("approved artist with music and search on approve off: %+v", j.Result)
	}
}

// "Search when approved" off: the catch-up follows the nightly switches, so
// with shows off too the approved show waits, as it did before.
func TestMissingSearchApprovedAskSearchOnApproveOff(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	setSetting(t, f.fixture, SettingMissingShows, "false")
	exec(t, f.fixture, `INSERT INTO settings (created_at, created_by, updated_at, updated_by, grp, key, value) VALUES ('t', '{}', 't', '{}', 'this_app', 'ask_search_on_approve', 'false')`)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("searched with search on approve and shows off: %+v", j.Result)
	}
}

// Caught up once: after the one-title search ran, the asked show is back in
// the normal turn, so the next scheduled search does not queue it again.
func TestMissingSearchApprovedAskCaughtUpOnce(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	sr := &Searcher{Store: f.s}
	// What the one-title search queues: season 1 together, S02E01 alone.
	runKind(t, f.fixture, sr, KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	runKind(t, f.fixture, sr, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S02E01"]}, nil)
	var never int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM items WHERE title_id = ? AND last_searched_at IS NULL AND (air_date IS NULL OR air_date < ?)", f.titleID, time.Now().UTC().Format(time.RFC3339)).Scan(&never)
	if never != 0 {
		t.Fatalf("%d aired episodes still never searched", never)
	}
	exec(t, f.fixture, "DELETE FROM jobs")
	j := runKind(t, f.fixture, sr, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("searched ask caught up again: %+v", j.Result)
	}
	// The show's episodes are still in the turn (oldest searched first).
	if episodeSearches(t, f) == 0 {
		t.Fatalf("the show left the normal turn: %+v", j.Result)
	}
}

// Only approved asks jump the queue; an ask still waiting or turned down
// does not, and neither does a bulk "better" search or one kept to movies.
func TestMissingSearchOnlyApprovedAsksCaughtUp(t *testing.T) {
	for _, state := range []string{"asked", "turned_down"} {
		ts := &tvServer{answer: func(url.Values) []string { return nil }}
		f := seedShow(t, ts.start(t), nil)
		askFor(t, f.fixture, f.titleID, state, "2026-10-31T17:07:51Z")
		j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
		if n, _ := j.Result["asks"].(float64); j.State != "done" || n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
			t.Fatalf("%s ask caught up: %s %+v", state, j.State, j.Result)
		}
	}
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	// A never-searched Slot that wants a better copy: a "better" search
	// still does not catch the ask up.
	exec(t, f.fixture, "UPDATE slots SET state = 'better_wanted' WHERE id = ?", f.slots["S02E01"])
	for _, p := range []map[string]any{{"which": "better"}, {"which": "missing", "kind": "movie"}} {
		j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, p)
		if n, _ := j.Result["asks"].(float64); j.State != "done" || n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
			t.Fatalf("%v caught the show up: %+v", p, j.Result)
		}
	}
	// A count of 0 turns the scheduled search off, catch-up included.
	setSetting(t, f.fixture, SettingMissingCount, "0")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("count 0 caught up: %+v", j.Result)
	}
}

// The catch-up stands in for the search approval starts, so the nightly
// "cover shows" switch does not stop it (as with that search, #139 review).
func TestMissingSearchApprovedAskIgnoresNightlyShowSwitch(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	setSetting(t, f.fixture, SettingMissingShows, "false")
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatalf("approved show with the nightly switch off: %+v", j.Result)
	}
}

// runAfters reads when the queued Jobs matching where may start, in order.
func runAfters(t *testing.T, f fixture, where string, args ...any) []time.Time {
	t.Helper()
	rows, err := f.s.Read().Query("SELECT run_after FROM jobs WHERE "+where+" ORDER BY run_after", args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		at, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, at)
	}
	return out
}

// Indexers ban bursts: the catch-up's titles go one after the other (each
// after the most searches the one before can queue, 2 s apart), and the
// normal turn after them all, so the titles do not ask the Sources at once.
func TestMissingSearchApprovedAsksSpreadOut(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	var movieTitle int64
	_ = f.s.Read().QueryRow("SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", f.slotID).Scan(&movieTitle)
	askFor(t, f.fixture, movieTitle, "approved", "2026-10-30T22:21:16Z")
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	// A third title in the normal turn, so the turn has something to queue.
	exec(t, f.fixture, `INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, gate_driver, name, sort_name, aliases, source_ids, metadata_source, keep_up_to_date, new_parts_policy, release_gate, music_types, why_added, artwork, status, state)
		SELECT created_at, created_by, updated_at, updated_by, kind, gate_driver, 'Other', 'other', aliases, '{"tmdb":"77"}', metadata_source, keep_up_to_date, new_parts_policy, release_gate, music_types, why_added, artwork, status, state FROM titles WHERE id = ?`, movieTitle)
	var other, ver, item int64
	_ = f.s.Read().QueryRow("SELECT max(id) FROM titles").Scan(&other)
	_ = f.s.Read().QueryRow("SELECT version_id FROM slots WHERE id = ?", f.slotID).Scan(&ver)
	exec(t, f.fixture, `INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, keep_up_to_date, source_ids)
		VALUES ('t', '{}', 't', '{}', ?, 'movie', 1, '{}')`, other)
	_ = f.s.Read().QueryRow("SELECT max(id) FROM items").Scan(&item)
	exec(t, f.fixture, `INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, keep_up_to_date, is_main)
		SELECT created_at, created_by, updated_at, updated_by, ?, label, pick_rules_id, library_folder_id, 'Other', keep_up_to_date, is_main FROM versions WHERE id = ?`, other, ver)
	_ = f.s.Read().QueryRow("SELECT max(id) FROM versions").Scan(&ver)
	exec(t, f.fixture, `INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES ('t', '{}', 't', '{}', ?, ?, 'wanted')`, item, ver)
	var otherSlot int64
	_ = f.s.Read().QueryRow("SELECT max(id) FROM slots").Scan(&otherSlot)
	before := time.Now().UTC()
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 2 {
		t.Fatalf("asks %+v", j.Result)
	}
	movie := runAfters(t, f.fixture, "dedupe_key = ?", "search_wanted:"+strconv.FormatInt(movieTitle, 10))
	show := runAfters(t, f.fixture, "dedupe_key = ?", "search_wanted:"+strconv.FormatInt(f.titleID, 10))
	turn := runAfters(t, f.fixture, "dedupe_key = ?", "search_slot:"+strconv.FormatInt(otherSlot, 10))
	if len(movie) != 1 || len(show) != 1 || len(turn) != 1 {
		t.Fatalf("jobs: movie %v show %v turn %v", movie, show, turn)
	}
	// The movie has one Slot: the show starts 2 s after it; the show has 4
	// wanted Slots: the turn starts 8 s after the show.
	if movie[0].Before(before.Add(-time.Second)) || show[0].Sub(movie[0]) < BulkStagger || turn[0].Sub(show[0]) < 4*BulkStagger {
		t.Fatalf("not spread out: movie %v, show %v, turn %v", movie[0], show[0], turn[0])
	}
}

// A season search that found nothing queues its episodes 2 s apart, not
// all at once.
func TestSeasonFallbackEpisodesSpreadOut(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	at := runAfters(t, f.fixture, "kind = ? AND json_extract(subject, '$.id') IN (?, ?, ?)", KindSearchSlot, f.slots["S01E01"], f.slots["S01E02"], f.slots["S01E03"])
	if len(at) != 3 {
		t.Fatalf("episode searches %v", at)
	}
	for i := 1; i < len(at); i++ {
		if at[i].Sub(at[i-1]) < BulkStagger-10*time.Millisecond {
			t.Fatalf("episode searches not spread out: %v", at)
		}
	}
}

// A few titles a run (askCatchUpPerRun), whatever missing_search_count:
// each is searched in full one after the other, so many would push the
// normal turn far out.
func TestMissingSearchApprovedAsksFewPerRun(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	var movieTitle int64
	_ = f.s.Read().QueryRow("SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", f.slotID).Scan(&movieTitle)
	askFor(t, f.fixture, movieTitle, "approved", "2026-10-30T22:21:16Z")
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	old := askCatchUpPerRun
	askCatchUpPerRun = 1
	defer func() { askCatchUpPerRun = old }()
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, movieTitle) != 1 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("one title a run: %+v", j.Result)
	}
	// The next run takes the next one.
	j = runKind(t, f.fixture, &Searcher{Store: f.s}, KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatalf("second run: %+v", j.Result)
	}
}
