package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// sourcesNamed counts the Sources other than id that carry the name, ignoring case.
func sourcesNamed(tx *sql.Tx, name string, id int64) (int, error) {
	var n int
	err := tx.QueryRow("SELECT count(*) FROM sources WHERE name = ? COLLATE NOCASE AND id != ?", name, id).Scan(&n)
	return n, err
}

// deleteSource removes a Source row; its Releases and calls go with it.
func deleteSource(tx *sql.Tx, id int64) error {
	_, err := tx.Exec("DELETE FROM sources WHERE id = ?", id)
	return err
}

// pauseSourceTx puts a Source on a back-off pause. Only the health columns
// change, read fresh in the same write, so a key or proxy edited meanwhile
// stays and a Source the person turned off stays off. It returns the new
// failure count, when the pause ends and whether the Source was paused.
func pauseSourceTx(tx *sql.Tx, ctx context.Context, sourceID int64, kind, detail string, wait time.Duration) (n int, until time.Time, paused bool, err error) {
	var failures sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT json_extract(extra, '$.failures') FROM sources WHERE id = ? AND state != 'disabled'`, sourceID).Scan(&failures)
	if err != nil {
		return 0, time.Time{}, false, err
	}
	n, _ = strconv.Atoi(failures.String)
	n++
	until = time.Now().UTC().Add(max(wait, Backoff(n)))
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `UPDATE sources SET state = 'paused', pause_reason = ?, paused_until = ?,
			extra = CASE WHEN ? = '' THEN json_remove(json_set(extra, '$.failures', ?), '$.block') ELSE json_set(extra, '$.failures', ?, '$.block', ?) END,
			updated_at = ?, updated_by = ? WHERE id = ? AND state != 'disabled'`,
		kind, database.FormatTime(until), detail, strconv.Itoa(n), strconv.Itoa(n), detail, database.FormatTime(now), systemActor(), sourceID)
	if err != nil {
		return n, until, false, err
	}
	k, _ := res.RowsAffected()
	return n, until, k == 1, nil
}

// sourceNeedsRecovery says whether a Source is paused or still carries a
// failure count or block note.
func sourceNeedsRecovery(ctx context.Context, q database.Querier, sourceID int64) (bool, error) {
	var need bool
	err := q.QueryRowContext(ctx, `SELECT state = 'paused' OR json_extract(extra, '$.failures') IS NOT NULL OR json_extract(extra, '$.block') IS NOT NULL
		OR json_extract(extra, '$.test_failed') IS NOT NULL FROM sources WHERE id = ?`,
		sourceID).Scan(&need)
	return need, err
}

// recoverSourceTx moves a paused Source back to enabled and clears its
// failure count. A Source the person turned off stays off.
func recoverSourceTx(tx *sql.Tx, ctx context.Context, sourceID int64) error {
	now := database.FormatTime(time.Now().UTC())
	if _, err := tx.ExecContext(ctx, `UPDATE sources SET state = 'enabled', pause_reason = NULL, paused_until = NULL,
			updated_at = ?, updated_by = ? WHERE id = ? AND state = 'paused'`, now, systemActor(), sourceID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE sources SET extra = json_remove(extra, '$.failures', '$.block', '$.test_failed'), updated_at = ?, updated_by = ?
			WHERE id = ? AND (json_extract(extra, '$.failures') IS NOT NULL OR json_extract(extra, '$.block') IS NOT NULL
			OR json_extract(extra, '$.test_failed') IS NOT NULL)`, now, systemActor(), sourceID)
	return err
}

// markTestFailedTx notes why a test of an enabled Source failed, and the
// kind of block when a site's protection stopped it. A paused or turned-off
// Source keeps the state it has.
func markTestFailedTx(tx *sql.Tx, ctx context.Context, sourceID int64, kind, detail string) error {
	_, err := tx.ExecContext(ctx, `UPDATE sources SET
			extra = CASE WHEN ? = '' THEN json_remove(json_set(extra, '$.test_failed', ?), '$.block') ELSE json_set(extra, '$.test_failed', ?, '$.block', ?) END,
			updated_at = ?, updated_by = ? WHERE id = ? AND state = 'enabled'`,
		detail, kind, kind, detail, database.FormatTime(time.Now().UTC()), systemActor(), sourceID)
	return err
}

func systemActor() string {
	b, _ := json.Marshal(domain.System)
	return string(b)
}

// missingFilter says which Slots a missing search looks at.
type missingFilter struct {
	States  []string
	Only    string // movie, show or artist; empty for every kind
	TitleID int64  // 0 for every Title
}

// from is the FROM and WHERE of the Slots that may be searched; arguments come from args.
func (f missingFilter) from() string {
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(f.States)), ", ")
	return ` FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id
		JOIN titles t ON t.id = i.title_id
		WHERE s.state IN (` + ph + `) AND (? = '' OR t.kind = ?) AND (? = 0 OR t.id = ?) AND i.keep_up_to_date = 1 AND v.keep_up_to_date = 1 AND t.state = 'active' AND s.active_download_id IS NULL
		AND (t.kind = 'movie' OR (? AND t.kind = 'show' AND t.keep_up_to_date = 1 AND s.state != 'not_out_yet' AND i.dropped_at IS NULL
			AND (i.part_id IS NULL OR EXISTS (SELECT 1 FROM parts p WHERE p.id = i.part_id AND p.keep_up_to_date = 1 AND p.dropped_at IS NULL)))
			OR (? AND t.kind = 'artist' AND t.keep_up_to_date = 1 AND s.state != 'not_out_yet' AND i.dropped_at IS NULL
			AND EXISTS (SELECT 1 FROM parts p WHERE p.id = i.part_id AND p.keep_up_to_date = 1 AND p.included = 1 AND p.dropped_at IS NULL)))`
}

func (f missingFilter) args(shows, music bool) []any {
	a := []any{}
	for _, st := range f.States {
		a = append(a, st)
	}
	return append(a, f.Only, f.Only, f.TitleID, f.TitleID, shows, music)
}

// askedTitle is a Title with an approved ask and how many Slots a search of it can queue.
type askedTitle struct{ id, slots int64 }

// approvedAskTitles lists the Titles with an approved ask and something never
// searched, oldest approval first, up to max. A Title is left out while its
// one-title search is queued or approval's own scan is still to run, and after
// a one-title search was queued since the approval.
func approvedAskTitles(ctx context.Context, db *sql.DB, f missingFilter, shows, music bool, max int) ([]askedTitle, error) {
	from := f.from()
	cq := `WITH c AS (SELECT a.title_id AS id, MIN(a.decided_at) AS d FROM asks a WHERE a.state = 'approved' AND a.title_id IS NOT NULL GROUP BY a.title_id),
			w AS (SELECT dedupe_key AS k, MAX(created_at) AS m FROM jobs WHERE dedupe_key LIKE 'search_wanted:%' GROUP BY dedupe_key)
			SELECT c.id, (SELECT count(*)` + from + ` AND t.id = c.id) FROM c LEFT JOIN w ON w.k = 'search_wanted:' || c.id
			WHERE (w.m IS NULL OR w.m < c.d) AND EXISTS (SELECT 1` + from + ` AND t.id = c.id AND i.last_searched_at IS NULL)
			AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.state IN ('queued', 'running') AND (j.dedupe_key = 'search_wanted:' || c.id
				OR (j.kind IN ('refresh_title', 'scan_title') AND json_extract(j.subject, '$.type') = 'title' AND json_extract(j.subject, '$.id') = c.id AND j.params LIKE '%search%')))
			ORDER BY c.d, c.id LIMIT ` + strconv.Itoa(max)
	ca := f.args(shows, music)
	rows, err := db.QueryContext(ctx, cq, append(append([]any{}, ca...), ca...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []askedTitle
	for rows.Next() {
		var a askedTitle
		if err := rows.Scan(&a.id, &a.slots); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// missingSlot is a Slot a missing search will queue a search for.
type missingSlot struct {
	id, part, ver int64
	kind          string
}

// missingSlots lists the Slots to search, least recently searched first, up
// to limit (a negative limit lists all), leaving out the Titles in skipTitles
// (given as id text) because their own search covers them.
func missingSlots(ctx context.Context, db *sql.DB, f missingFilter, shows, music bool, skipTitles []string, limit int) ([]missingSlot, error) {
	q := `SELECT s.id, t.kind, COALESCE(i.part_id, 0), s.version_id` + f.from()
	if len(skipTitles) > 0 {

		// Their own search covers them, so the cap goes to the rest.
		q += ` AND t.id NOT IN (` + strings.Join(skipTitles, ", ") + `)`
	}
	q += ` ORDER BY i.last_searched_at IS NOT NULL, i.last_searched_at, s.id`
	if limit >= 0 {
		q += " LIMIT " + strconv.Itoa(limit)
	}
	rows, err := db.QueryContext(ctx, q, f.args(shows, music)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []missingSlot
	for rows.Next() {
		var s missingSlot
		if err := rows.Scan(&s.id, &s.kind, &s.part, &s.ver); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// refuseHeldDangerousTx refuses a Release that was still held when its files
// turned out to be a program, and returns how many rows changed.
func refuseHeldDangerousTx(tx *sql.Tx, releaseID int64, now time.Time, by domain.Actor) (int64, error) {
	res, err := tx.Exec(`UPDATE releases SET state = 'refused', hold_until = NULL,
					verdict = json_set(COALESCE(verdict, '{}'), '$.outcome', 'refused', '$.dangerous_files', json('true')), updated_at = ?, updated_by = ? WHERE id = ? AND state = 'held'`,
		database.FormatTime(now), actorText(by), releaseID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TagUses counts how many times each tag is set on a Source. A row saved
// without tags may hold JSON null rather than a list, so only lists count.
func TagUses(ctx context.Context, db *sql.DB) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, "SELECT j.value FROM sources x, json_each(x.tags) j WHERE json_type(x.tags) = 'array'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		out[label]++
	}
	return out, rows.Err()
}
