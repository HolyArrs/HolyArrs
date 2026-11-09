package sources

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// askFor stores an ask on the Title in the given state and decision time, adding the user it needs.
func askFor(t *testing.T, f fixture, titleID int64, state, decided string) {
	t.Helper()
	exec(t, f, `INSERT OR IGNORE INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'ana', 'member', 'x')`)
	exec(t, f, `INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at, decided_at)
		SELECT 't', '{}', 't', '{}', t.kind, CAST(t.id AS TEXT), t.name, ?, t.id, 1, 't', ? FROM titles t WHERE t.id = ?`, state, decided, titleID)
}

// wantedJobs counts the one-Title missing searches queued for the Title.
func wantedJobs(t *testing.T, f fixture, titleID int64) int {
	t.Helper()
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND dedupe_key = ? AND json_extract(params, '$.title_id') = ?",
		jobs.KindSearchMissing, "search_wanted:"+strconv.FormatInt(titleID, 10), titleID).Scan(&n)
	return n
}

// episodeSearches counts the season and episode searches queued for the show.
func episodeSearches(t *testing.T, f showFix) int {
	t.Helper()
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind IN (?, ?) AND (json_extract(params, '$.version_id') = ? OR json_extract(subject, '$.id') = ?)",
		jobs.KindSearchSeason, jobs.KindSearchSlot, f.verID, f.slots["S02E01"]).Scan(&n)
	return n
}

func TestHandleSearchMissing_ApprovedAskFirst(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	setSetting(t, f.fixture, SettingMissingCount, "1")
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")

	exec(t, f.fixture, "UPDATE items SET last_searched_at = '2020-01-01T00:00:00Z' WHERE id = (SELECT item_id FROM slots WHERE id = ?)", f.slotID)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["asks"].(float64); n != 1 {
		t.Fatalf("asks caught up: %+v", j.Result)
	}
	if wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatal("the asked show did not get its one-title search")
	}

	var movie int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') = ?", jobs.KindSearchSlot, f.slotID).Scan(&movie)

	n, _ := j.Result["slots"].(float64)
	queued, _ := j.Result["queued"].(float64)
	if n != 1 || queued != 1 || movie != 1 {
		t.Fatalf("rest of the cap: %+v, movie searches %d", j.Result, movie)
	}
}

func TestHandleSearchMissing_ApprovedAskNotRepeated(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	sr := &Searcher{Store: f.s}

	fresh := func() {
		exec(t, f.fixture, "DELETE FROM jobs")
		exec(t, f.fixture, "UPDATE items SET last_searched_at = NULL")
	}
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-11-01T04:00:00Z', '{}', 't', '{}', ?, '{"type":"title","id":1}', '{}', 'normal', 'done', 1, 5, 't', '{}', ?)`,
		jobs.KindSearchMissing, "search_wanted:"+strconv.FormatInt(f.titleID, 10))
	j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["asks"].(float64); n != 0 {
		t.Fatalf("caught up twice: %+v", j.Result)
	}

	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-10-01T04:00:00Z', '{}', 't', '{}', ?, '{"type":"title","id":1}', '{}', 'normal', 'done', 1, 5, 't', '{}', ?)`,
		jobs.KindSearchMissing, "search_wanted:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 {
		t.Fatalf("a search from before the approval blocked the catch-up: %+v", j.Result)
	}

	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-10-01T04:00:00Z', '{}', 't', '{}', ?, '{"type":"title","id":1}', '{}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}', ?)`,
		jobs.KindSearchMissing, "search_wanted:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 {
		t.Fatalf("caught up while its one-title search was queued: %+v", j.Result)
	}

	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-11-04T04:00:00Z', '{}', 't', '{}', 'scan_title', ?, '{"then":["search_wanted"]}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}', ?)`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`, "scan_then_search:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("caught up while approval's scan was queued: %+v", j.Result)
	}

	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by)
		VALUES ('2026-11-04T04:00:00Z', '{}', 't', '{}', 'refresh_title', ?, '{"then":["scan_title","search_wanted"]}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}')`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`)
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("caught up while approval's refresh was queued: %+v", j.Result)
	}

	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-10-01T04:00:00Z', '{}', 't', '{}', 'scan_title', ?, '{"then":["search_wanted"]}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}', ?)`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`, "scan_title:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("caught up while the chain's scan was queued: %+v", j.Result)
	}

	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-11-04T04:00:00Z', '{}', 't', '{}', 'refresh_title', ?, '{}', 'normal', 'queued', 0, 5, '2099-01-01T00:00:00Z', '{}', ?)`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`, "refresh_title:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 {
		t.Fatalf("a plain refresh held the catch-up back: %+v", j.Result)
	}

	fresh()
	exec(t, f.fixture, `INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, subject, params, priority, state, attempt, max_attempts, run_after, asked_by, dedupe_key)
		VALUES ('2026-11-04T04:00:00Z', '{}', 't', '{}', 'scan_title', ?, '{"then":["search_wanted"]}', 'normal', 'failed', 5, 5, 't', '{}', ?)`,
		`{"type":"title","id":`+strconv.FormatInt(f.titleID, 10)+`}`, "scan_then_search:"+strconv.FormatInt(f.titleID, 10))
	j = runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatalf("a failed approval scan blocked the catch-up: %+v", j.Result)
	}
}

func TestHandleSearchMissing_ApprovedAsksOldestFirstAndCapped(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	var movieTitle int64
	_ = f.s.Read().QueryRow("SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", f.slotID).Scan(&movieTitle)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	askFor(t, f.fixture, movieTitle, "approved", "2026-10-30T22:21:16Z")
	setSetting(t, f.fixture, SettingMissingCount, "1")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, movieTitle) != 1 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("older approval first, one title: %+v movie %d show %d", j.Result, wantedJobs(t, f.fixture, movieTitle), wantedJobs(t, f.fixture, f.titleID))
	}
}

func TestHandleSearchMissing_ApprovedArtistMusicSwitch(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return nil }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', ?, 'false', '2026-01-01', '{}', '2026-01-01', '{}')`, SettingMissingMusic)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T01:33:30Z")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatalf("approved artist with music off: %+v", j.Result)
	}
	exec(t, f.fixture, "DELETE FROM jobs")
	exec(t, f.fixture, "UPDATE items SET last_searched_at = NULL")
	exec(t, f.fixture, `INSERT INTO settings (created_at, created_by, updated_at, updated_by, grp, key, value) VALUES ('t', '{}', 't', '{}', 'this_app', 'ask_search_on_approve', 'false')`)
	j = runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("approved artist with music and search on approve off: %+v", j.Result)
	}
}

func TestHandleSearchMissing_ApprovedAskSearchOnApproveOff(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	setSetting(t, f.fixture, SettingMissingShows, "false")
	exec(t, f.fixture, `INSERT INTO settings (created_at, created_by, updated_at, updated_by, grp, key, value) VALUES ('t', '{}', 't', '{}', 'this_app', 'ask_search_on_approve', 'false')`)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("searched with search on approve and shows off: %+v", j.Result)
	}
}

func TestHandleSearchMissing_ApprovedAskCaughtUpOnce(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	sr := &Searcher{Store: f.s}

	runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S02E01"]}, nil)
	var never int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM items WHERE title_id = ? AND last_searched_at IS NULL AND (air_date IS NULL OR air_date < ?)", f.titleID, time.Now().UTC().Format(time.RFC3339)).Scan(&never)
	if never != 0 {
		t.Fatalf("%d aired episodes still never searched", never)
	}
	exec(t, f.fixture, "DELETE FROM jobs")
	j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("searched ask caught up again: %+v", j.Result)
	}

	if episodeSearches(t, f) == 0 {
		t.Fatalf("the show left the normal turn: %+v", j.Result)
	}
}

func TestHandleSearchMissing_OnlyApprovedAsksCaughtUp(t *testing.T) {
	for _, state := range []string{"asked", "turned_down"} {
		ts := &tvServer{answer: func(url.Values) []string { return nil }}
		f := seedShow(t, ts.start(t), nil)
		askFor(t, f.fixture, f.titleID, state, "2026-10-31T17:07:51Z")
		j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
		if n, _ := j.Result["asks"].(float64); j.State != "done" || n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
			t.Fatalf("%s ask caught up: %s %+v", state, j.State, j.Result)
		}
	}
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")

	exec(t, f.fixture, "UPDATE slots SET state = 'better_wanted' WHERE id = ?", f.slots["S02E01"])
	for _, p := range []map[string]any{{"which": "better"}, {"which": "missing", "kind": "movie"}} {
		j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, p)
		if n, _ := j.Result["asks"].(float64); j.State != "done" || n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
			t.Fatalf("%v caught the show up: %+v", p, j.Result)
		}
	}

	setSetting(t, f.fixture, SettingMissingCount, "0")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 0 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("count 0 caught up: %+v", j.Result)
	}
}

func TestHandleSearchMissing_ApprovedAskIgnoresNightlyShowSwitch(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	setSetting(t, f.fixture, SettingMissingShows, "false")
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatalf("approved show with the nightly switch off: %+v", j.Result)
	}
}

// runAfters returns the start times of the Jobs matching the where clause, oldest first.
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

func TestHandleSearchMissing_ApprovedAsksSpreadOut(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	var movieTitle int64
	_ = f.s.Read().QueryRow("SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", f.slotID).Scan(&movieTitle)
	askFor(t, f.fixture, movieTitle, "approved", "2026-10-30T22:21:16Z")
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")

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
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 2 {
		t.Fatalf("asks %+v", j.Result)
	}
	movie := runAfters(t, f.fixture, "dedupe_key = ?", "search_wanted:"+strconv.FormatInt(movieTitle, 10))
	show := runAfters(t, f.fixture, "dedupe_key = ?", "search_wanted:"+strconv.FormatInt(f.titleID, 10))
	turn := runAfters(t, f.fixture, "dedupe_key = ?", "search_slot:"+strconv.FormatInt(otherSlot, 10))
	if len(movie) != 1 || len(show) != 1 || len(turn) != 1 {
		t.Fatalf("jobs: movie %v show %v turn %v", movie, show, turn)
	}

	if movie[0].Before(before.Add(-time.Second)) || show[0].Sub(movie[0]) < BulkStagger || turn[0].Sub(show[0]) < 4*BulkStagger {
		t.Fatalf("not spread out: movie %v, show %v, turn %v", movie[0], show[0], turn[0])
	}
}

func TestHandleSearchMissing_SeasonFallbackEpisodesSpreadOut(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	at := runAfters(t, f.fixture, "kind = ? AND json_extract(subject, '$.id') IN (?, ?, ?)", jobs.KindSearchSlot, f.slots["S01E01"], f.slots["S01E02"], f.slots["S01E03"])
	if len(at) != 3 {
		t.Fatalf("episode searches %v", at)
	}
	for i := 1; i < len(at); i++ {
		if at[i].Sub(at[i-1]) < BulkStagger-10*time.Millisecond {
			t.Fatalf("episode searches not spread out: %v", at)
		}
	}
}

func TestHandleSearchMissing_ApprovedAsksFewPerRun(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	var movieTitle int64
	_ = f.s.Read().QueryRow("SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", f.slotID).Scan(&movieTitle)
	askFor(t, f.fixture, movieTitle, "approved", "2026-10-30T22:21:16Z")
	askFor(t, f.fixture, f.titleID, "approved", "2026-10-31T17:07:51Z")
	old := askCatchUpPerRun
	askCatchUpPerRun = 1
	defer func() { askCatchUpPerRun = old }()
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, movieTitle) != 1 || wantedJobs(t, f.fixture, f.titleID) != 0 {
		t.Fatalf("one title a run: %+v", j.Result)
	}

	j = runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if n, _ := j.Result["asks"].(float64); n != 1 || wantedJobs(t, f.fixture, f.titleID) != 1 {
		t.Fatalf("second run: %+v", j.Result)
	}
}
