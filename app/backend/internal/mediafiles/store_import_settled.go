package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
)

// waitingSlotsSQL picks the Slots in the JSON list ?2 (the download's
// Slots) that still have no file
// and are still wanted: what a partly done import leaves waiting.
const waitingSlotsSQL = `SELECT s.id FROM slots s JOIN items it ON it.id = s.item_id LEFT JOIN parts p ON p.id = it.part_id
	WHERE s.id IN (SELECT value FROM json_each(?2))
	AND s.file_id IS NULL AND it.keep_up_to_date = 1 AND (p.id IS NULL OR p.keep_up_to_date = 1)
	AND s.state NOT IN ('in_library', 'removed', 'rejected')
	ORDER BY s.id`

// unfilledSlotsSQL picks the Slots in ?2 with no file that are not in the
// Library, removed or rejected. A failed import holds its download until
// these are gone: the files may still be imported by hand, so an episode
// that is merely unmonitored keeps the hold (movies are never monitored
// that way).
const unfilledSlotsSQL = `SELECT s.id FROM slots s WHERE s.id IN (SELECT value FROM json_each(?2))
	AND s.file_id IS NULL AND s.state NOT IN ('in_library', 'removed', 'rejected')
	ORDER BY s.id`

// ImportSettled reports whether the newest import of the download is done
// and nothing about it is still at work or waiting for a person: no import
// planned, approved or placing, and no Slot of it importing or needing a
// decision. A partly done import counts as settled once none of its
// episodes still waits: a later import filled them, or the person
// stopped wanting them. A failed import is settled once every Slot of it
// was filled from elsewhere, removed or rejected, so its download is not
// held forever. slotIDs are the download's Slots.
func ImportSettled(ctx context.Context, db *sql.DB, downloadID int64, slotIDs []int64) (bool, error) {
	var ok bool
	err := db.QueryRowContext(ctx, `SELECT COALESCE(
		CASE (SELECT i.state FROM imports i WHERE i.download_id = ?1 ORDER BY i.id DESC LIMIT 1)
			WHEN 'done' THEN 1
			WHEN 'partly_done' THEN NOT EXISTS (`+waitingSlotsSQL+`)
			WHEN 'failed' THEN json_array_length(?2) > 0 AND NOT EXISTS (`+unfilledSlotsSQL+`)
			ELSE 0 END
		AND NOT EXISTS (SELECT 1 FROM imports i WHERE i.download_id = ?1 AND i.state IN ('planned', 'waiting_for_you', 'approved', 'placing'))
		AND NOT EXISTS (SELECT 1 FROM slots s WHERE s.active_download_id = ?1 AND s.state IN ('importing', 'needs_a_decision')), 0)`,
		downloadID, slotList(slotIDs)).Scan(&ok)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return ok, err
}

// ImportRunning reports whether HolyArrs is placing the download's files
// right now: an import approved or placing, or a Slot of it importing.
func ImportRunning(ctx context.Context, db *sql.DB, downloadID int64) (bool, error) {
	var ok bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM imports WHERE download_id = ?1 AND state IN ('approved', 'placing'))
		OR EXISTS (SELECT 1 FROM slots WHERE active_download_id = ?1 AND state = 'importing')`, downloadID).Scan(&ok)
	return ok, err
}

// ImportsAtWork counts imports placing files right now and Slots importing;
// an in-app update waits until both are zero.
func ImportsAtWork(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM imports WHERE state = 'placing') +
		(SELECT COUNT(*) FROM slots WHERE state = 'importing')`).Scan(&n)
	return n, err
}

// CleanupWaiting lists, per download whose newest import is partly done or failed,
// the Slots still waiting for a file: seeding cleanup holds the download
// until they are filled or no longer wanted. slots maps each download to
// its Slots. Downloads with nothing waiting are left out.
func CleanupWaiting(ctx context.Context, db *sql.DB, slots map[int64][]int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	for id, slotIDs := range slots {
		var st string
		err := db.QueryRowContext(ctx, `SELECT state FROM imports WHERE download_id = ? ORDER BY id DESC LIMIT 1`, id).Scan(&st)
		if err == sql.ErrNoRows || err == nil && st != "partly_done" && st != "failed" {
			continue
		}
		if err != nil {
			return out, err
		}
		q := waitingSlotsSQL
		if st == "failed" {
			q = unfilledSlotsSQL
		}
		rows, err := db.QueryContext(ctx, q, id, slotList(slotIDs))
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var s int64
			if err := rows.Scan(&s); err != nil {
				rows.Close()
				return out, err
			}
			out[id] = append(out[id], s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, err
		}
	}
	return out, nil
}

// slotList is the JSON list the waiting-Slots query reads.
func slotList(ids []int64) string {
	if ids == nil {
		ids = []int64{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}
