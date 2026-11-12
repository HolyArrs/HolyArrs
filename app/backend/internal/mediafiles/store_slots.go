package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// EventNeedsDecision is the event kind written when a Slot moves to
// needs_a_decision.
const EventNeedsDecision = "needs_decision"

// MoveSlot moves a Slot to a new state, refusing moves that model.md
// section 4 does not allow. reason is kept as the Slot's last_reason.
func MoveSlot(ctx context.Context, s *database.Store, slotID int64, to SlotState, reason string, by database.Actor) error {
	return s.Write(ctx, func(tx *sql.Tx) error { return MoveSlotTx(tx, slotID, to, reason, by) })
}

// MoveSlotTx is MoveSlot inside a write transaction the caller already holds.
func MoveSlotTx(tx *sql.Tx, slotID int64, to SlotState, reason string, by database.Actor) error {
	var from string
	if err := tx.QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&from); err != nil {
		if err == sql.ErrNoRows {
			return database.ErrNotFound
		}
		return err
	}
	if err := Transition(SlotState(from), to); err != nil {
		return err
	}
	var r any
	if reason != "" {
		r = reason
	}
	_, err := tx.Exec("UPDATE slots SET state = ?, last_reason = ?, updated_at = ?, updated_by = ? WHERE id = ?",
		string(to), r, database.FormatTime(time.Now()), actorJSON(by), slotID)
	if err == nil && to == SlotNeedsADecision && from != string(to) {
		needsDecision(tx, slotID, reason, by)
	}
	return err
}

// needsDecision writes a needs_decision Event once the move to
// needs_a_decision has committed, so History and Alerts hear of it.
func needsDecision(tx *sql.Tx, slotID int64, reason string, by database.Actor) {
	s := database.StoreOf(tx)
	if s == nil {
		return
	}
	var titleID int64
	var name string
	_ = tx.QueryRow("SELECT t.id, t.name FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE s.id = ?", slotID).Scan(&titleID, &name)
	database.AfterCommit(tx, func() {
		e := &database.Event{Actor: by, Kind: EventNeedsDecision, SlotID: &slotID, Summary: "Needs a decision: " + name,
			Data: map[string]any{"reason": reason}, GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
		if titleID > 0 {
			e.TitleID = &titleID
		}
		if err := s.AddEvent(context.Background(), e); err != nil {
			log.Printf("needs a decision event: %v", err)
		}
	})
}

func actorJSON(a database.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}

// ClearSlotDownloadTx unties a Slot from its download: the download is let go.
func ClearSlotDownloadTx(tx *sql.Tx, slotID int64) error {
	_, err := tx.Exec("UPDATE slots SET active_download_id = NULL WHERE id = ?", slotID)
	return err
}

// FollowedDownloads lists the downloads a Slot still follows: downloading,
// stalled, importing or waiting on a decision. Such a download is not over.
func FollowedDownloads(ctx context.Context, q database.Lister) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT active_download_id FROM slots WHERE active_download_id IS NOT NULL
		AND state IN ('downloading', 'stalled', 'importing', 'needs_a_decision')`)
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
