package sources

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// slotsOfDownloadInSeason counts the Slots a Download filled in one Part
// (season) for one Version.
func slotsOfDownloadInSeason(ctx context.Context, q database.Querier, downloadID, partID, versionID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM downloads d, json_each(d.slot_ids) j JOIN slots s ON s.id = j.value
		JOIN items i ON i.id = s.item_id WHERE d.id = ? AND i.part_id = ? AND s.version_id = ?`, downloadID, partID, versionID).Scan(&n)
	return n, err
}

// collectLinksWith adds to links the id and link of each Release of the
// Source whose link holds key.
func collectLinksWith(ctx context.Context, tx *sql.Tx, sourceID int64, key string, links map[int64]string) error {
	rows, err := tx.QueryContext(ctx, "SELECT id, link_ref FROM releases WHERE source_id = ? AND instr(link_ref, ?) > 0", sourceID, key)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var link string
		if err := rows.Scan(&id, &link); err != nil {
			rows.Close()
			return err
		}
		links[id] = link
	}
	rows.Close()
	return rows.Err()
}

// setReleaseLink replaces a Release's link.
func setReleaseLink(ctx context.Context, tx *sql.Tx, id int64, link string) error {
	_, err := tx.ExecContext(ctx, "UPDATE releases SET link_ref = ? WHERE id = ?", link, id)
	return err
}

// turnedDownList selects Releases on the Turned down list: turned down whole, or for some Slots only.
const turnedDownList = "(state = 'turned_down' OR json_extract(verdict, '$.turned_down_slot_ids') IS NOT NULL)"

// markChosenTx marks a Release chosen for a Slot and adds the Slot to the ones it matched.
func markChosenTx(tx *sql.Tx, releaseID, slotID int64, by domain.Actor) error {
	_, err := tx.Exec(`UPDATE releases SET state = 'chosen', hold_until = NULL,
			matched_slot_ids = CASE WHEN EXISTS (SELECT 1 FROM json_each(matched_slot_ids) WHERE value = ?) THEN matched_slot_ids
				ELSE json_insert(matched_slot_ids, '$[#]', ?) END,
			updated_at = ?, updated_by = ? WHERE id = ?`, slotID, slotID, database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

// markTurnedDownTx puts a Release on the Turned down list with the reason.
func markTurnedDownTx(tx *sql.Tx, releaseID int64, reason string, by domain.Actor) error {
	_, err := tx.Exec(`UPDATE releases SET state = 'turned_down', hold_until = NULL, verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?),
			updated_at = ?, updated_by = ? WHERE id = ?`, reason, database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

// setTurnedDownSlotsTx replaces the Slots a Release is turned down for, and clears the list when empty.
func setTurnedDownSlotsTx(tx *sql.Tx, releaseID int64, ids []int64) error {
	if len(ids) == 0 {
		_, err := tx.Exec("UPDATE releases SET verdict = json_remove(verdict, '$.turned_down_slot_ids') WHERE id = ?", releaseID)
		return err
	}
	_, err := tx.Exec("UPDATE releases SET verdict = json_set(verdict, '$.turned_down_slot_ids', json(?)) WHERE id = ?", jsonText(ids), releaseID)
	return err
}

// heldReleaseIDsTx lists the held Releases, or just id when it is above 0.
func heldReleaseIDsTx(tx *sql.Tx, id int64) ([]int64, error) {
	q := "SELECT id FROM releases WHERE state = 'held'"
	args := []any{}
	if id > 0 {
		q += " AND id = ?"
		args = append(args, id)
	}
	rows, err := tx.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var rid int64
		if err := rows.Scan(&rid); err != nil {
			return nil, err
		}
		ids = append(ids, rid)
	}
	return ids, rows.Err()
}

// markRemovedByHandTx passes over a Release a person removed from the Held list.
func markRemovedByHandTx(tx *sql.Tx, releaseID int64, by domain.Actor) error {
	_, err := tx.Exec(`UPDATE releases SET state = 'passed_over', hold_until = NULL, verdict = json_set(COALESCE(verdict, '{}'), '$.removed_by_hand', json('true')),
				updated_at = ?, updated_by = ? WHERE id = ?`, database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

// removeTurnedDownTx takes Releases off the Turned down list (one when id is
// above 0, else all) and returns how many it changed. Releases with a Download
// or a send attempt are kept as passed over or refused so their history stays;
// the others are deleted.
func removeTurnedDownTx(tx *sql.Tx, id int64, by domain.Actor) (int, error) {
	where := "state = 'turned_down'"
	args := []any{}
	if id > 0 {
		where += " AND id = ?"
		args = append(args, id)
	}
	n := 0
	res, err := tx.Exec(`UPDATE releases SET verdict = json_remove(verdict, '$.turned_down_slot_ids'), updated_at = ?, updated_by = ?
			WHERE state != 'turned_down' AND json_extract(verdict, '$.turned_down_slot_ids') IS NOT NULL`+strings.TrimPrefix(where, "state = 'turned_down'"),
		append([]any{database.FormatTime(time.Now()), actorText(by)}, args...)...)
	if err != nil {
		return 0, err
	}
	forgot, _ := res.RowsAffected()
	n += int(forgot)
	used := " AND (EXISTS (SELECT 1 FROM downloads d WHERE d.release_id = releases.id) OR EXISTS (SELECT 1 FROM downloader_attempts a WHERE a.release_id = releases.id))"

	res, err = tx.Exec(`UPDATE releases SET state = CASE WHEN json_extract(verdict, '$.outcome') = 'refused' AND EXISTS (SELECT 1 FROM json_each(verdict, '$.reasons') rr
				WHERE json_extract(rr.value, '$.effect') = 'fail' AND NOT (json_extract(rr.value, '$.step') = 'hard' AND json_extract(rr.value, '$.rule') = 'turned_down'))
				OR EXISTS (SELECT 1 FROM downloads dd WHERE dd.release_id = releases.id AND dd.state = 'failed' AND dd.state_reason LIKE 'dangerous files%')
				THEN 'refused' ELSE 'passed_over' END,
			verdict = json_set(COALESCE(verdict, '{}'), '$.taken_off_turned_down', json('true')),
			updated_at = ?, updated_by = ? WHERE `+where+used, append([]any{database.FormatTime(time.Now()), actorText(by)}, args...)...)
	if err != nil {
		return 0, err
	}
	kept, _ := res.RowsAffected()
	res, err = tx.Exec("DELETE FROM releases WHERE "+where, args...)
	if err != nil {
		return 0, err
	}
	k, _ := res.RowsAffected()
	n += int(k + kept)
	return n, nil
}

// listReleases returns a page of Releases in a state, newest first (held ones
// by wait end), and the total before paging.
func listReleases(ctx context.Context, db *sql.DB, state string, limit, offset int) ([]domain.Release, int, error) {
	var total int
	where, args := "state = ?", []any{state}
	if state == "turned_down" {
		where, args = turnedDownList, nil
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM releases WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "updated_at DESC, id DESC"
	if state == "held" {
		order = "hold_until, id"
	}
	rels, err := database.Select[domain.Release](ctx, db, "releases", where+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(args, limit, offset)...)
	return rels, total, err
}

// sourceName is the name of a Source, "" when it is gone.
func sourceName(ctx context.Context, q database.Querier, id int64) string {
	var name string
	_ = q.QueryRowContext(ctx, "SELECT name FROM sources WHERE id = ?", id).Scan(&name)
	return name
}

// dangerousDownloadOf says whether a Download of the Release failed for dangerous files.
func dangerousDownloadOf(ctx context.Context, q database.Querier, releaseID int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM downloads WHERE release_id = ? AND state = 'failed' AND state_reason LIKE 'dangerous files%'", releaseID).Scan(&n)
	return n > 0
}

// dangerousPair is the hash and Release name of a Download that failed for dangerous files.
type dangerousPair struct{ hash, name string }

// dangerousDownloads lists every Download that failed for dangerous files.
func dangerousDownloads(ctx context.Context, db *sql.DB) ([]dangerousPair, error) {
	rows, err := db.QueryContext(ctx, `SELECT lower(d.external_id), COALESCE(r.name, '') FROM downloads d LEFT JOIN releases r ON r.id = d.release_id
		WHERE d.state = 'failed' AND d.state_reason LIKE 'dangerous files%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dangerousPair
	for rows.Next() {
		var p dangerousPair
		if rows.Scan(&p.hash, &p.name) == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

// updateJudgedTx records a new verdict on a Release that is already stored:
// its state follows the verdict unless it is chosen, its verdict for this
// Slot is merged in, and the Slots it matched are added to.
func updateJudgedTx(tx *sql.Tx, releaseID int64, state string, hold *time.Time, v mediafiles.Verdict, bySlot map[string]any, by domain.Actor) error {
	_, err := tx.Exec(`UPDATE releases SET state = CASE WHEN state = 'chosen' THEN state ELSE ? END,
					hold_until = CASE WHEN state = 'chosen' THEN hold_until ELSE ? END,
					verdict = json_set(json(?), '$.by_slot', json_patch(COALESCE(json_extract(verdict, '$.by_slot'), '{}'), json(?))),
					matched_slot_ids = (SELECT json_group_array(value) FROM (SELECT value FROM json_each(matched_slot_ids) UNION SELECT value FROM json_each(?))),
					updated_at = ?, updated_by = ? WHERE id = ?`,
		state, timeOrNil(hold), jsonText(toMap(v)), jsonText(bySlot), jsonText(nonNil(v.SlotIDs)), database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

// insertReleaseTx stores a new Release and returns its id.
func insertReleaseTx(tx *sql.Tx, rel *domain.Release, by domain.Actor) (int64, error) {
	return database.Insert(tx, "releases", rel, by)
}

// releaseStateTx reads a Release's state in the caller's transaction.
func releaseStateTx(tx *sql.Tx, releaseID int64) (string, error) {
	var st string
	err := tx.QueryRow("SELECT state FROM releases WHERE id = ?", releaseID).Scan(&st)
	return st, err
}

// sendAbandonedTx says whether sending the Release for the Slot's Version
// was given up on before.
func sendAbandonedTx(tx *sql.Tx, slotID, releaseID int64) (bool, error) {
	var abandoned bool
	err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM downloader_attempts a
			JOIN releases r ON r.id = a.release_id AND r.created_at = a.release_created_at
			JOIN versions v ON v.id = a.version_id AND v.created_at = a.version_created_at
			JOIN slots s ON s.version_id = v.id WHERE s.id = ? AND r.id = ? AND a.state = 'abandoned')`, slotID, releaseID).Scan(&abandoned)
	return abandoned, err
}

// passOverOpenTx passes over a Release that is still judged or held.
func passOverOpenTx(tx *sql.Tx, releaseID int64, by domain.Actor) error {
	_, err := tx.Exec(`UPDATE releases SET state = 'passed_over', hold_until = NULL, updated_at = ?, updated_by = ?
			WHERE id = ? AND state IN ('judged', 'held')`, database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

// passOverHeldReleaseTx passes over a Release that is held.
func passOverHeldReleaseTx(tx *sql.Tx, releaseID int64, by domain.Actor) error {
	_, err := tx.Exec(`UPDATE releases SET state = 'passed_over', hold_until = NULL, updated_at = ?, updated_by = ?
			WHERE id = ? AND state = 'held'`, database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

// turnedDownNames lists the names and info hashes of Releases turned down
// for the Slot, whole or for it alone.
func turnedDownNames(ctx context.Context, db *sql.DB, slotID int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, COALESCE(info_hash, '') FROM releases
		WHERE state = 'turned_down' AND EXISTS (SELECT 1 FROM json_each(COALESCE(json_extract(verdict, '$.turned_down_slot_ids'), matched_slot_ids))
			WHERE value = ?) OR EXISTS (SELECT 1 FROM json_each(json_extract(verdict, '$.turned_down_slot_ids')) WHERE value = ?)`, slotID, slotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n, h string
		if err := rows.Scan(&n, &h); err != nil {
			return nil, err
		}
		out = append(out, n)
		if h != "" {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}

// markPickedTx marks a Release chosen or held for its Slots. The Slots it
// fills are added to the verdict's pick_slot_ids, which the Download reads;
// Slots of another Version picked for the same Release stay listed.
func markPickedTx(tx *sql.Tx, releaseID int64, state string, until *time.Time, slotIDs []int64, by domain.Actor) error {
	_, err := tx.Exec(`UPDATE releases SET state = ?, hold_until = ?,
		verdict = json_set(COALESCE(verdict, '{}'), '$.pick_slot_ids', (SELECT json_group_array(value) FROM (
			SELECT value FROM json_each(COALESCE(json_extract(verdict, '$.pick_slot_ids'), '[]')) UNION SELECT value FROM json_each(?)))),
		matched_slot_ids = (SELECT json_group_array(value) FROM (SELECT value FROM json_each(matched_slot_ids) UNION SELECT value FROM json_each(?))),
		updated_at = ?, updated_by = ? WHERE id = ?`,
		state, timeOrNil(until), jsonText(slotIDs), jsonText(slotIDs), database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

// slotFilter narrows a Title's Slots by one column: cond is the SQL (using
// the aliases s for the Slot and i for the Item) and id its argument.
type slotFilter struct {
	cond string
	id   int64
}

// latestSearchJobs lists the newest search Job of each Slot of a Title: one
// whose Releases matched the Slot, or a search of the Slot itself, so Releases
// that matched no Title are listed too.
func latestSearchJobs(ctx context.Context, db *sql.DB, titleID int64, filters []slotFilter) ([]any, error) {
	slotFilterSQL := ""
	var slotArg []any
	for _, f := range filters {
		slotFilterSQL += f.cond
		slotArg = append(slotArg, f.id)
	}
	args := append(append([]any{titleID}, slotArg...), append([]any{jobs.KindSearchSlot, titleID}, slotArg...)...)
	args = append(args, append([]any{jobs.KindSearchSeason, jobs.KindSearchAlbum, titleID}, slotArg...)...)
	rows, err := db.QueryContext(ctx, `SELECT max(job_id) FROM (
		SELECT s.id AS slot, r.job_id AS job_id FROM releases r JOIN json_each(r.matched_slot_ids) j
			JOIN slots s ON s.id = j.value JOIN items i ON i.id = s.item_id
			WHERE i.title_id = ? AND r.job_id IS NOT NULL AND r.found_by != 'feed'`+slotFilterSQL+`
		UNION ALL
		SELECT s.id, jb.id FROM jobs jb JOIN slots s ON s.id = json_extract(jb.subject, '$.id') JOIN items i ON i.id = s.item_id
			WHERE jb.kind = ? AND json_extract(jb.subject, '$.type') = 'slot' AND i.title_id = ?`+slotFilterSQL+`
			AND EXISTS (SELECT 1 FROM releases r WHERE r.job_id = jb.id)
		UNION ALL
		SELECT s.id, jb.id FROM jobs jb JOIN items i ON i.part_id = json_extract(jb.subject, '$.id')
			JOIN slots s ON s.item_id = i.id AND s.version_id = json_extract(jb.params, '$.version_id')
			WHERE jb.kind IN (?, ?) AND json_extract(jb.subject, '$.type') = 'part' AND i.title_id = ?`+slotFilterSQL+`
			AND EXISTS (SELECT 1 FROM releases r WHERE r.job_id = jb.id)
	) GROUP BY slot`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobIDs []any
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		jobIDs = append(jobIDs, id)
	}
	return jobIDs, nil
}

// releasesOfJobs counts the Releases the Jobs found and returns one page of
// them: winners first, then passed over, then refused.
func releasesOfJobs(ctx context.Context, db *sql.DB, jobIDs []any, limit, offset int) ([]domain.Release, int, error) {
	in := "job_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(jobIDs)), ",") + ")"
	var total int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM releases WHERE "+in, jobIDs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rels, err := database.Select[domain.Release](ctx, db, "releases", in+` ORDER BY CASE state WHEN 'chosen' THEN 0 WHEN 'held' THEN 1
			WHEN 'judged' THEN 2 WHEN 'passed_over' THEN 3 WHEN 'turned_down' THEN 5 ELSE 4 END,
			COALESCE(json_extract(verdict, '$.rank'), 9999), id LIMIT ? OFFSET ?`, append(jobIDs, limit, offset)...)
	return rels, total, err
}

// runningSearchJobs lists the ids of the queued or running search Jobs of a Title.
func runningSearchJobs(ctx context.Context, db *sql.DB, titleID int64) []int64 {
	js, err := database.Select[domain.Job](ctx, db, "jobs", `state IN ('queued', 'running') AND (
		kind = ? AND json_extract(subject, '$.type') = 'slot'
			AND json_extract(subject, '$.id') IN (SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?)
		OR kind IN (?, ?) AND json_extract(subject, '$.type') = 'part' AND json_extract(subject, '$.id') IN (SELECT id FROM parts WHERE title_id = ?)) ORDER BY id`,
		jobs.KindSearchSlot, titleID, jobs.KindSearchSeason, jobs.KindSearchAlbum, titleID)
	running := []int64{}
	if err == nil {
		for _, j := range js {
			running = append(running, j.ID)
		}
	}
	return running
}

var searchKinds = []any{jobs.KindSearchSlot, jobs.KindSearchMissing, jobs.KindFeedCheck, jobs.KindSearchSeason, jobs.KindSearchAlbum}

// searchJobs lists the search Jobs queued or running, oldest first, and the
// newest finished ones up to limit.
func searchJobs(ctx context.Context, db *sql.DB, limit int) (running, recent []domain.Job, err error) {
	kinds := "kind IN (?, ?, ?, ?, ?)"
	running, err = database.Select[domain.Job](ctx, db, "jobs", kinds+" AND state IN ('queued', 'running') ORDER BY id LIMIT 100", searchKinds...)
	if err != nil {
		return nil, nil, err
	}
	recent, err = database.Select[domain.Job](ctx, db, "jobs", kinds+" AND state IN ('done', 'failed') AND ended_at IS NOT NULL ORDER BY ended_at DESC, id DESC LIMIT ?",
		append(searchKinds, limit)...)
	return running, recent, err
}

// releaseName is the name of a Release, "" when it is gone.
func releaseName(ctx context.Context, q database.Querier, id int64) string {
	var name string
	_ = q.QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", id).Scan(&name)
	return name
}
