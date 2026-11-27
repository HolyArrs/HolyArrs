package sources

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Releases a Slot waits on: grabbing them, and freeing Slots whose wait failed.

// freeFailedHeld sends Slots back to looking when the Release they wait
// on was judged again and no longer passes. held maps each waiting Slot to
// its Release; cands and vs are what was just judged.
func (s *Searcher) freeFailedHeld(ctx context.Context, by domain.Actor, held map[int64]int64, cands []found, vs []mediafiles.Verdict) error {
	freed := map[int64]bool{}
	for _, relID := range held {
		if freed[relID] {
			continue
		}
		for k := range cands {
			if cands[k].relID != relID || vs[k].Passed {
				continue
			}
			freed[relID] = true
			if err := s.Store.Write(ctx, func(tx *sql.Tx) error { return unholdTx(tx, relID, "Held Release no longer passes", by) }); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

// grabHeld chooses the Held Releases whose wait is over and returns how many Slots were settled.
func (s *Searcher) grabHeld(ctx context.Context, by domain.Actor, jobID *int64, now time.Time) (int, error) {
	slots, err := mediafiles.HeldSlotsDue(ctx, s.Store.Read(), now)
	if err != nil {
		return 0, err
	}
	n := 0
	done := map[int64]bool{}
	for _, sl := range slots {
		if done[sl.ID] {
			continue
		}
		var rel domain.Release
		if err := database.Get(ctx, s.Store.Read(), "releases", *sl.HeldReleaseID, &rel); err != nil {
			continue
		}
		// The Release stopped waiting without this Slot (chosen for another, or passed over): its Slots go back to looking.
		if rel.State != "held" {
			_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
				if st, err := releaseStateTx(tx, rel.ID); err != nil || st == "held" {
					return err
				}
				return unholdTx(tx, rel.ID, "Held Release is no longer waiting", by)
			})
			continue
		}
		// The files turned out to be a program during the wait: refuse the Release and send its Slots back to looking.
		if s.dangerousRel(ctx, rel) {

			_ = s.Store.Write(ctx, func(tx *sql.Tx) error {

				// Only while it is still held, since a person may have chosen it meanwhile.
				n, err := refuseHeldDangerousTx(tx, rel.ID, now, by)
				if err != nil || n == 0 {
					return err
				}
				return unholdTx(tx, rel.ID, "not sent: "+dangerousWhy, by)
			})
			done[sl.ID] = true
			continue
		}

		// The show's year or names changed during the wait, or the Release waited from before the year was checked.
		if s.heldShowMiss(ctx, sl, rel) {
			_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
				if err := passOverHeldReleaseTx(tx, rel.ID, by); err != nil {
					return err
				}
				return unholdTx(tx, rel.ID, "Held Release no longer passes", by)
			})
			done[sl.ID] = true
			continue
		}
		if !s.heldStillTaken(ctx, sl.ID, rel) {
			s.dropHeldNotTaken(ctx, sl.ID, rel, by)
			done[sl.ID] = true
			continue
		}

		// A season pack or album held for several Slots is chosen for all on its pick list that still hold it.
		// A Release with no pick list (one left waiting by an older build) is chosen for every due Slot of this Version that holds it.
		// A Slot missing from a list that names others is chosen alone.
		pick := []int64{sl.ID}
		many := PickSlots(rel)
		listed, _ := rel.Verdict["pick_slot_ids"].([]any)
		if len(many) > 1 && slices.Contains(many, sl.ID) || len(listed) == 0 {
			pick = pick[:0]
			for _, o := range slots {
				if o.HeldReleaseID != nil && *o.HeldReleaseID == rel.ID && (slices.Contains(many, o.ID) || len(listed) == 0 && o.VersionID == sl.VersionID) {
					pick = append(pick, o.ID)
				}
			}
		}
		for _, id := range pick {
			done[id] = true
		}
		if err := s.settleChosenMany(ctx, pick, rel.ID, rel.Name, by); errors.Is(err, ErrBusy) {
			continue
		} else if err != nil {
			return n, err
		}
		n++
		titleID, _ := mediafiles.ItemTitleID(ctx, s.Store.Read(), sl.ItemID)
		for _, id := range pick {
			s.addEvent(ctx, by, jobID, titleID, id, EventReleaseChosen, "Wait over, chose "+rel.Name, map[string]any{"release_id": rel.ID, "was_held": true})
			s.publish("slot", map[string]any{"id": id, "state": mediafiles.SlotChosen, "reason": "chose " + rel.Name})
		}
		if s.OnChosen != nil {
			s.OnChosen(ctx, sl.ID, rel.ID)
		}
	}
	return n, nil
}

// freeUnfilledHeld sends a waiting Slot back to looking when the Release it
// waits on was judged again, still passes, but no longer fills that Slot.
// held maps each waiting Slot to its Release; cands and vs are what was just judged.
func (s *Searcher) freeUnfilledHeld(ctx context.Context, by domain.Actor, held map[int64]int64, cands []found, vs []mediafiles.Verdict) error {
	for slotID, relID := range held {
		for k := range cands {
			if cands[k].relID != relID || !vs[k].Passed || slices.Contains(vs[k].SlotIDs, slotID) {
				continue
			}
			if err := s.Store.Write(ctx, func(tx *sql.Tx) error { return unholdTx(tx, relID, "Held Release no longer fills this", by, slotID) }); err != nil {
				return err
			}
			break
		}
	}
	return nil
}
