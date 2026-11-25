package sources

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// KindCleanReleases is the daily Job that removes old search results.
const KindCleanReleases = "clean_releases"

// ReleaseKeepDays is how long a Release nobody needs stays after its last
// change. The old apps keep search results only in memory and drop pending
// ones after about a month; two weeks keeps the last searches to look at.
const ReleaseKeepDays = 14

// releasePruneBatch is how many Releases one write removes, so the main
// database is never held for long. A large first run goes batch by batch.
const releasePruneBatch = 5000

// keptReleaseRefs is the part of a removal check that only the main
// database can answer: no Download, send attempt or Slot points at the
// Release, and nothing marks it turned down for a Slot. It is checked again
// inside each delete, so a Release grabbed meanwhile stays.
const keptReleaseRefs = `r.state IN ('seen', 'judged', 'passed_over', 'refused') AND r.updated_at < ?
	AND json_extract(r.verdict, '$.turned_down_slot_ids') IS NULL
	AND NOT EXISTS (SELECT 1 FROM downloads d WHERE d.release_id = r.id)
	AND NOT EXISTS (SELECT 1 FROM downloader_attempts a WHERE a.release_id = r.id)
	AND NOT EXISTS (SELECT 1 FROM slots s WHERE s.held_release_id = r.id)
	AND NOT EXISTS (SELECT 1 FROM slots s WHERE s.chosen_release_id = r.id)`

// PruneReleases removes Releases older than ReleaseKeepDays that nothing
// needs, a batch per write, and returns how many went. It keeps chosen,
// held and turned-down Releases, any with a Download, a send attempt or a
// History Event naming it, and every Release of each Slot's newest search,
// so the Releases sheet still shows the last search and why one was picked.
func PruneReleases(ctx context.Context, st *database.Store, now time.Time) (int, error) {
	cutoff := database.FormatTime(now.Add(-ReleaseKeepDays * 24 * time.Hour))
	inHistory, err := releasesInHistory(ctx, st.ReadEvents())
	if err != nil {
		return 0, err
	}
	newest, err := newestSearchJobs(ctx, st.Read())
	if err != nil {
		return 0, err
	}
	total, after := 0, int64(0)
	for {
		ids, last, err := releaseCandidates(ctx, st.Read(), cutoff, after)
		if err != nil {
			return total, err
		}
		if last == 0 {
			return total, nil
		}
		after = last
		var drop []any
		for _, c := range ids {
			if !inHistory[c.id] && !(c.job.Valid && newest[c.job.Int64]) {
				drop = append(drop, c.id)
			}
		}
		if len(drop) > 0 {
			var n int64
			err := st.Write(ctx, func(tx *sql.Tx) error {
				res, err := tx.Exec(`DELETE FROM releases WHERE id IN (SELECT r.id FROM releases r WHERE r.id IN (?`+strings.Repeat(", ?", len(drop)-1)+`) AND `+keptReleaseRefs+`)`,
					append(drop, cutoff)...)
				if err == nil {
					n, err = res.RowsAffected()
				}
				return err
			})
			total += int(n)
			if err != nil {
				return total, err
			}
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}

type releaseCandidate struct {
	id  int64
	job sql.NullInt64
}

// releaseCandidates reads the next batch of old Releases after id after
// that the main database lets go, oldest id first; last is the highest id
// read (0: none left).
func releaseCandidates(ctx context.Context, db *sql.DB, cutoff string, after int64) ([]releaseCandidate, int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT r.id, r.job_id FROM releases r WHERE r.id > ? AND `+keptReleaseRefs+` ORDER BY r.id LIMIT ?`, after, cutoff, releasePruneBatch)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []releaseCandidate
	var last int64
	for rows.Next() {
		var c releaseCandidate
		if err := rows.Scan(&c.id, &c.job); err != nil {
			return nil, 0, err
		}
		out, last = append(out, c), c.id
	}
	return out, last, rows.Err()
}

// releasesInHistory lists the Releases History Events name.
func releasesInHistory(ctx context.Context, db *sql.DB) (map[int64]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT CAST(json_extract(data, '$.release_id') AS INTEGER) FROM events
		WHERE json_valid(data) AND json_extract(data, '$.release_id') IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// newestSearchJobs lists each Slot's newest search Job, the same way the
// Releases sheet picks it (latestSearchJobsSQL), for every Slot at once.
func newestSearchJobs(ctx context.Context, db *sql.DB) (map[int64]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT max(job_id) FROM (
		SELECT j.value AS slot, r.job_id AS job_id FROM releases r JOIN json_each(r.matched_slot_ids) j
			WHERE r.job_id IS NOT NULL AND r.found_by != 'feed'
		UNION ALL
		SELECT json_extract(jb.subject, '$.id'), jb.id FROM jobs jb
			WHERE jb.kind = ? AND json_extract(jb.subject, '$.type') = 'slot'
			AND EXISTS (SELECT 1 FROM releases r WHERE r.job_id = jb.id)
		UNION ALL
		SELECT s.id, jb.id FROM jobs jb JOIN items i ON i.part_id = json_extract(jb.subject, '$.id')
			JOIN slots s ON s.item_id = i.id AND s.version_id = json_extract(jb.params, '$.version_id')
			WHERE jb.kind IN (?, ?) AND json_extract(jb.subject, '$.type') = 'part'
			AND EXISTS (SELECT 1 FROM releases r WHERE r.job_id = jb.id)
	) GROUP BY slot`, jobs.KindSearchSlot, jobs.KindSearchSeason, jobs.KindSearchAlbum)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id.Valid {
			out[id.Int64] = true
		}
	}
	return out, rows.Err()
}
