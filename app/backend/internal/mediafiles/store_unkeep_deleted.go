package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// UnkeptDeleted is what UnkeepDeletedTx switched off.
type UnkeptDeleted struct {
	TitleID int64
	Kind    string  // movie or show
	Name    string  // the title's name
	Items   []int64 // the episodes switched off (shows only)
	Version bool    // only the movie's Version whose file went was switched off
}

// UnkeepDeletedTx turns Keep up to date off for what a File gone from disk
// filled, as a person would by hand: the episodes it held, or its whole
// movie. A Slot still looking for a better copy stops looking. ok is false
// when the File filled nothing that was still kept up to date.
func UnkeepDeletedTx(ctx context.Context, tx *sql.Tx, fileID int64, now time.Time, by database.Actor) (UnkeptDeleted, bool, error) {
	var u UnkeptDeleted
	var titleKeep bool
	err := tx.QueryRowContext(ctx, `SELECT t.id, t.kind, t.name, t.keep_up_to_date FROM slots s JOIN items i ON i.id = s.item_id
		JOIN titles t ON t.id = i.title_id WHERE s.file_id = ? AND t.kind IN ('movie', 'show') LIMIT 1`, fileID).Scan(&u.TitleID, &u.Kind, &u.Name, &titleKeep)
	if errors.Is(err, sql.ErrNoRows) {
		return u, false, nil
	}
	if err != nil {
		return u, false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT s.item_id FROM slots s JOIN items i ON i.id = s.item_id
		WHERE s.file_id = ? AND i.keep_up_to_date = 1 ORDER BY s.item_id`, fileID)
	if err != nil {
		return u, false, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return u, false, err
		}
		u.Items = append(u.Items, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return u, false, err
	}
	at, who := database.FormatTime(now), actorJSON(by)
	// A movie's switch is the title's; it covers its items and Versions too,
	// as the title edit does. A show keeps one switch per episode.
	ids, _ := json.Marshal(u.Items)
	slotsOf, slotArg := "SELECT id FROM slots WHERE state = 'better_wanted' AND item_id IN (SELECT value FROM json_each(?))", any(string(ids))
	if u.Kind == "movie" {
		if !titleKeep && len(u.Items) == 0 {
			return u, false, nil
		}
		// A movie with another Version still on disk keeps its switch: only
		// the Version whose file went stops being kept up to date, so the
		// copies the owner still has are left alone.
		var other int
		err := tx.QueryRowContext(ctx, `SELECT count(*) FROM slots s JOIN items i ON i.id = s.item_id JOIN files f ON f.id = s.file_id
			WHERE i.title_id = ? AND s.file_id != ? AND f.role = 'main' AND f.state = 'present'`, u.TitleID, fileID).Scan(&other)
		if err != nil {
			return u, false, err
		}
		if other > 0 {
			res, err := tx.ExecContext(ctx, `UPDATE versions SET keep_up_to_date = 0, updated_at = ?, updated_by = ?
				WHERE keep_up_to_date = 1 AND id IN (SELECT version_id FROM slots WHERE file_id = ?)`, at, who, fileID)
			if err != nil {
				return u, false, err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return u, false, nil
			}
			u.Version = true
			slotsOf, slotArg = "SELECT id FROM slots WHERE state = 'better_wanted' AND file_id = ?", fileID
		}
		for _, q := range []string{"UPDATE titles SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE id = ?",
			"UPDATE items SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE title_id = ?",
			"UPDATE versions SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE title_id = ?"} {
			if u.Version {
				break
			}
			if _, err := tx.ExecContext(ctx, q, at, who, u.TitleID); err != nil {
				return u, false, err
			}
		}
		if !u.Version {
			slotsOf, slotArg = "SELECT id FROM slots WHERE state = 'better_wanted' AND item_id IN (SELECT id FROM items WHERE title_id = ?)", u.TitleID
		}
		u.Items = nil
	} else {
		if len(u.Items) == 0 {
			return u, false, nil
		}
		if _, err := tx.ExecContext(ctx, "UPDATE items SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE id IN (SELECT value FROM json_each(?))",
			at, who, string(ids)); err != nil {
			return u, false, err
		}
	}
	srows, err := tx.QueryContext(ctx, slotsOf, slotArg)
	if err != nil {
		return u, false, err
	}
	var slots []int64
	for srows.Next() {
		var id int64
		if err := srows.Scan(&id); err != nil {
			srows.Close()
			return u, false, err
		}
		slots = append(slots, id)
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return u, false, err
	}
	for _, id := range slots {
		if err := MoveSlotTx(tx, id, SlotInLibrary, "its file was deleted from disk, so Keep up to date was turned off", by); err != nil {
			return u, false, err
		}
	}
	return u, true, nil
}
