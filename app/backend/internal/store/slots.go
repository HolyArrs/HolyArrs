package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// MoveSlot moves a Slot to a new state, refusing moves that model.md
// section 4 does not allow. reason is kept as the Slot's last_reason.
func (s *Store) MoveSlot(ctx context.Context, slotID int64, to domain.SlotState, reason string, by domain.Actor) error {
	return s.Write(ctx, func(tx *sql.Tx) error { return MoveSlotTx(tx, slotID, to, reason, by) })
}

// MoveSlotTx is MoveSlot inside a write transaction the caller already holds.
func MoveSlotTx(tx *sql.Tx, slotID int64, to domain.SlotState, reason string, by domain.Actor) error {
	var from string
	if err := tx.QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&from); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if err := domain.Transition(domain.SlotState(from), to); err != nil {
		return err
	}
	var r any
	if reason != "" {
		r = reason
	}
	_, err := tx.Exec("UPDATE slots SET state = ?, last_reason = ?, updated_at = ?, updated_by = ? WHERE id = ?",
		string(to), r, fmtTime(time.Now()), actorJSON(by), slotID)
	if err == nil && to == domain.SlotNeedsADecision && from != string(to) {
		needsDecision(tx, slotID, reason, by)
	}
	return err
}

// needsDecision writes a needs_decision Event once the move to
// needs_a_decision has committed, so History and Alerts hear of it.
func needsDecision(tx *sql.Tx, slotID int64, reason string, by domain.Actor) {
	s := storeOf(tx)
	if s == nil {
		return
	}
	var titleID int64
	var name string
	_ = tx.QueryRow("SELECT t.id, t.name FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE s.id = ?", slotID).Scan(&titleID, &name)
	AfterCommit(tx, func() {
		e := &domain.Event{Actor: by, Kind: domain.EventNeedsDecision, SlotID: &slotID, Summary: "Needs a decision: " + name,
			Data: map[string]any{"reason": reason}, GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
		if titleID > 0 {
			e.TitleID = &titleID
		}
		if err := s.AddEvent(context.Background(), e); err != nil {
			log.Printf("needs a decision event: %v", err)
		}
	})
}

func actorJSON(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}
