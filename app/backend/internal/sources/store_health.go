package sources

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// searchCallsSince counts the searches asked of a Source after since.
func searchCallsSince(ctx context.Context, q database.Querier, sourceID int64, since string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM source_calls WHERE source_id = ? AND op = 'search' AND at > ?", sourceID, since).Scan(&n)
	return n, err
}

// picksSince counts the Releases of a Source picked after since: those that
// reached a Download and those chosen but not yet downloaded.
func picksSince(ctx context.Context, q database.Querier, sourceID int64, since string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM downloads d JOIN releases r ON r.id = d.release_id WHERE r.source_id = ? AND d.created_at > ?) +
		(SELECT count(*) FROM releases r WHERE r.source_id = ? AND r.state = 'chosen' AND r.updated_at > ?
			AND NOT EXISTS (SELECT 1 FROM downloads d WHERE d.release_id = r.id))`, sourceID, since, sourceID, since).Scan(&n)
	return n, err
}

// storeCall writes one call to a Source and drops the calls older than the
// keep time and the oldest ones past maxCallsPerSource.
func storeCall(ctx context.Context, s *database.Store, sourceID int64, op string, took time.Duration, ok bool, errorKind string, now time.Time) {
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO source_calls(source_id, at, op, ok, ms, error_kind) VALUES (?, ?, ?, ?, ?, ?)",
			sourceID, database.FormatTime(now), op, ok, took.Milliseconds(), errorKind); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM source_calls WHERE source_id = ? AND at < ?", sourceID, database.FormatTime(now.Add(-keepCalls))); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM source_calls WHERE source_id = ? AND id <= (SELECT id FROM source_calls WHERE source_id = ?
			ORDER BY id DESC LIMIT 1 OFFSET ?)`, sourceID, sourceID, maxCallsPerSource)
		return err
	})
}

// callSummaries reads the calls since day for every Source, by id; the last
// success and failure are looked for since kept.
func callSummaries(ctx context.Context, db *sql.DB, kept, day string) (map[int64]Summary, error) {
	rows, err := db.QueryContext(ctx, `SELECT s.id,
		(SELECT max(at) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ? AND c.ok = 1),
		(SELECT max(at) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ? AND c.ok = 0),
		(SELECT count(*) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ?),
		(SELECT count(*) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ? AND c.ok = 0),
		(SELECT COALESCE(CAST(avg(ms) AS INTEGER), 0) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ? AND c.ok = 1)
		FROM sources s`, kept, kept, day, day, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Summary{}
	for rows.Next() {
		var id int64
		var x Summary
		if err := rows.Scan(&id, &x.LastSuccess, &x.LastFailure, &x.Calls, &x.Failures, &x.AvgMS); err != nil {
			return nil, err
		}
		out[id] = x
	}
	return out, rows.Err()
}

// callsPerDay counts a Source's calls, failures and average time of
// successes for each day from the given day on.
func callsPerDay(ctx context.Context, db *sql.DB, sourceID int64, from string) ([]Day, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(at, 1, 10) d, count(*), count(CASE WHEN ok = 0 THEN 1 END),
		COALESCE(CAST(avg(CASE WHEN ok = 1 THEN ms END) AS INTEGER), 0)
		FROM source_calls WHERE source_id = ? AND at >= ? GROUP BY d`, sourceID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Day
	for rows.Next() {
		var x Day
		if err := rows.Scan(&x.Day, &x.Calls, &x.Failures, &x.AvgMS); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

// grabsPerDay counts the Releases of a Source chosen on each day from the
// given day on.
func grabsPerDay(ctx context.Context, db *sql.DB, sourceID int64, from string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(updated_at, 1, 10) d, count(*) FROM releases
		WHERE source_id = ? AND state = 'chosen' AND updated_at >= ? GROUP BY d`, sourceID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		out[d] = n
	}
	return out, nil
}

// recentCalls lists a Source's last 20 calls, newest first.
func recentCalls(ctx context.Context, db *sql.DB, sourceID int64) ([]Call, error) {
	rows, err := db.QueryContext(ctx, `SELECT at, op, ok, ms, error_kind FROM source_calls WHERE source_id = ? ORDER BY at DESC, id DESC LIMIT 20`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Call{}
	for rows.Next() {
		var c Call
		if err := rows.Scan(&c.At, &c.Op, &c.OK, &c.MS, &c.ErrorKind); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// releaseSeen says whether a Release of that name from the Source came from
// the feed or is still waiting to be picked. A feed Release that was refused
// is not seen: seeders, age and size change, so each feed check judges it
// again. Its id comes back so the new verdict updates the same record.
func releaseSeen(ctx context.Context, q database.Querier, sourceID int64, name string) (bool, int64, error) {
	var seen bool
	var refused int64
	err := q.QueryRowContext(ctx, `SELECT
				EXISTS (SELECT 1 FROM releases WHERE source_id = ? AND name = ?
					AND ((found_by = 'feed' AND state <> 'refused') OR state = 'judged')),
				COALESCE((SELECT MAX(id) FROM releases WHERE source_id = ? AND name = ?
					AND found_by = 'feed' AND state = 'refused'), 0)`,
		sourceID, name, sourceID, name).Scan(&seen, &refused)
	return seen, refused, err
}
