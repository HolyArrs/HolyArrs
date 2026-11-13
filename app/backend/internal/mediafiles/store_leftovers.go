package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Leftover is a present video of a title that fills no listed episode:
// a found note, or a file linked only to episodes the source no longer
// lists. DroppedSeason is the old season of such episodes (-1 for a note).
type Leftover struct {
	File          File
	DroppedSeason int
}

// TitleLeftovers lists a title's leftovers, by path.
func TitleLeftovers(ctx context.Context, q database.Lister, titleID int64) ([]Leftover, error) {
	fs, err := database.Select[File](ctx, q, "files", `state = 'present' AND role = 'main'
		AND version_id IN (SELECT id FROM versions WHERE title_id = ?)
		AND (origin = 'found' OR (EXISTS (SELECT 1 FROM slots s WHERE s.file_id = files.id)
			AND NOT EXISTS (SELECT 1 FROM slots s JOIN items i ON i.id = s.item_id WHERE s.file_id = files.id AND i.dropped_at IS NULL)))
		ORDER BY relative_path`, titleID)
	if err != nil {
		return nil, err
	}
	out := make([]Leftover, 0, len(fs))
	for _, f := range fs {
		l := Leftover{File: f, DroppedSeason: -1}
		if f.Origin != "found" {
			rows, err := q.QueryContext(ctx, `SELECT coalesce(min(json_extract(i.numbers, '$.season')), -1) FROM slots s JOIN items i ON i.id = s.item_id WHERE s.file_id = ?`, f.ID)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				_ = rows.Scan(&l.DroppedSeason)
			}
			_ = rows.Close()
		}
		out = append(out, l)
	}
	return out, nil
}

// EpisodeSlot is the Slot of one episode in a Version, and whether the
// source still lists the episode.
type EpisodeSlot struct {
	Slot    Slot
	Dropped bool
}

// ErrNotThisVersion says an episode is not one of the Version's.
var ErrNotThisVersion = errors.New("the episode is not one of this title's")

// EpisodeSlotTx reads the Slot of episode itemID in the Version.
func EpisodeSlotTx(ctx context.Context, tx *sql.Tx, versionID, itemID int64) (EpisodeSlot, error) {
	var es EpisodeSlot
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT s.id, i.dropped_at IS NOT NULL FROM slots s JOIN items i ON i.id = s.item_id
		WHERE s.version_id = ? AND s.item_id = ? AND i.kind = 'episode'`, versionID, itemID).Scan(&id, &es.Dropped)
	if errors.Is(err, sql.ErrNoRows) {
		return es, ErrNotThisVersion
	}
	if err != nil {
		return es, err
	}
	return es, database.Get(ctx, tx, "slots", id, &es.Slot)
}

// LinkFileInPlaceTx makes a File fill slotIDs where it lies: the Slots it
// held before are let go, a dropped episode nothing holds any more is
// removed (holding is the SQL that says a Slot holds its episode), and
// the File stops being a leftover.
func LinkFileInPlaceTx(ctx context.Context, tx *sql.Tx, fileID int64, slotIDs []int64, goodEnough bool, state SlotState, why, holding string, now time.Time, by database.Actor) error {
	at := database.FormatTime(now)
	olds, err := slotItems(ctx, tx, fileID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE slots SET file_id = NULL, good_enough = 0, state = 'wanted', updated_at = ?, updated_by = ? WHERE file_id = ?`, at, actorJSON(by), fileID); err != nil {
		return err
	}
	ids, _ := json.Marshal(slotIDs)
	if _, err := tx.ExecContext(ctx, `UPDATE files SET slot_ids = json(?), origin = 'adopted', leftover_reason = NULL, leftover_dismissed = 0, updated_at = ?, updated_by = ? WHERE id = ?`,
		string(ids), at, actorJSON(by), fileID); err != nil {
		return err
	}
	for _, sid := range slotIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE slots SET file_id = ?, good_enough = ?, state = ?, last_reason = ?, held_release_id = NULL, updated_at = ?, updated_by = ? WHERE id = ? AND state != 'removed'`,
			fileID, goodEnough, string(state), why, at, actorJSON(by), sid); err != nil {
			return err
		}
	}
	for _, it := range olds {
		if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id = ? AND dropped_at IS NOT NULL AND NOT EXISTS (SELECT 1 FROM slots WHERE item_id = items.id AND (`+holding+`))`, it); err != nil {
			return err
		}
	}
	return nil
}

// UnlinkFileTx turns a File back into a found note with the reason: the
// Slots it filled are empty again. Nothing on disk changes.
func UnlinkFileTx(ctx context.Context, tx *sql.Tx, fileID int64, reason string, now time.Time, by database.Actor) error {
	at := database.FormatTime(now)
	if _, err := tx.ExecContext(ctx, `UPDATE slots SET file_id = NULL, good_enough = 0, state = 'wanted', last_reason = ?, updated_at = ?, updated_by = ? WHERE file_id = ?`,
		"file matched by hand elsewhere", at, actorJSON(by), fileID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE files SET slot_ids = '[]', origin = 'found', leftover_reason = ?, updated_at = ?, updated_by = ? WHERE id = ?`, reason, at, actorJSON(by), fileID)
	return err
}

// UnlinkVersionFilesTx turns every main File of the Version that fills a
// Slot, but keep, into a found note with the reason.
func UnlinkVersionFilesTx(ctx context.Context, tx *sql.Tx, versionID, keep int64, reason string, now time.Time, by database.Actor) error {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT f.id FROM files f JOIN slots s ON s.file_id = f.id WHERE f.version_id = ? AND f.id != ? AND f.role = 'main'`, versionID, keep)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	for _, id := range ids {
		if err := UnlinkFileTx(ctx, tx, id, reason, now, by); err != nil {
			return err
		}
	}
	return nil
}

// SetLeftoverReason stores why a found File fills no episode.
func SetLeftoverReason(ctx context.Context, q database.Execer, fileID int64, reason string) error {
	_, err := q.ExecContext(ctx, `UPDATE files SET leftover_reason = ? WHERE id = ? AND origin = 'found' AND leftover_reason IS NOT ?`, reason, fileID, reason)
	return err
}

// SetLeftoverDismissed marks a leftover as left alone, or not.
func SetLeftoverDismissed(ctx context.Context, q database.Execer, fileID int64, dismissed bool) error {
	_, err := q.ExecContext(ctx, `UPDATE files SET leftover_dismissed = ? WHERE id = ?`, dismissed, fileID)
	return err
}

func slotItems(ctx context.Context, tx *sql.Tx, fileID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT item_id FROM slots WHERE file_id = ?`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
