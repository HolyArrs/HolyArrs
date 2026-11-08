package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// settlePicks settles show picks, choosing or holding each release for all its
// Slots in one write. It returns how many were chosen and held, and the Slots
// they took.
func (s *Searcher) settlePicks(ctx context.Context, by domain.Actor, jobID *int64, titleID int64, labels map[int64]string, all []found, vs []mediafiles.Verdict, ids []int64,
	picks []showPick, how string) (int, int, map[int64]bool) {
	chosen, held := 0, 0
	took := map[int64]bool{}
	for _, pk := range picks {
		name := all[pk.idx].item.Title
		var err error
		if pk.held {
			err = s.settleHeldMany(ctx, pk.slots, ids[pk.idx], name, *vs[pk.idx].HoldUntil, by)
		} else {
			err = s.settleChosenMany(ctx, pk.slots, ids[pk.idx], name, by)
		}
		if errors.Is(err, ErrBusy) || err != nil {
			continue
		}
		eps := make([]string, 0, len(pk.slots))
		for _, id := range pk.slots {
			took[id] = true
			if l := labels[id]; l != "" && !slices.Contains(eps, l) {
				eps = append(eps, l)
			}
		}
		data := map[string]any{"release_id": ids[pk.idx], "slot_ids": pk.slots, "episodes": eps}
		for _, id := range pk.slots {
			if pk.held {
				s.addEvent(ctx, by, jobID, titleID, id, EventReleaseHeld, fmt.Sprintf("%s holding %s for %s", how, name, strings.Join(eps, ", ")), data)
				s.publish("slot", map[string]any{"id": id, "state": mediafiles.SlotHeld, "reason": "holding " + name})
			} else {
				s.addEvent(ctx, by, jobID, titleID, id, EventReleaseChosen, fmt.Sprintf("%s chose %s for %s", how, name, strings.Join(eps, ", ")), data)
				s.publish("slot", map[string]any{"id": id, "state": mediafiles.SlotChosen, "reason": "chose " + name})
			}
		}
		if pk.held {
			held++
			continue
		}
		chosen++
		if s.OnChosen != nil {
			s.OnChosen(ctx, pk.slots[0], ids[pk.idx])
		}
	}
	return chosen, held, took
}

// settleChosenMany makes a release the pick of several Slots in one write, as
// for a season pack: one release, one Download, many Slots. Every Slot is read
// again inside the write, and when one is no longer looking it returns ErrBusy
// and nothing changes.
func (s *Searcher) settleChosenMany(ctx context.Context, slotIDs []int64, releaseID int64, name string, by domain.Actor) error {
	if len(slotIDs) == 0 {
		return errors.New("no Slot to choose the Release for")
	}
	if len(slotIDs) == 1 {
		return s.settleChosen(ctx, slotIDs[0], releaseID, name, by)
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range slotIDs {
			from, err := choiceStateTx(tx, id)
			if err != nil {
				return err
			}
			if !slices.Contains(pickStates, from) {
				return busyErr(from)
			}
		}
		for _, id := range slotIDs {
			if err := moveSlotTx(tx, id, mediafiles.SlotChosen, "chose "+name, by, releaseID); err != nil {
				return err
			}
			if err := passOverHeldTx(tx, id, releaseID, by); err != nil {
				return err
			}
		}
		return pickReleaseTx(tx, releaseID, "chosen", nil, slotIDs, by)
	})
	if errors.Is(err, ErrBusy) {
		s.lostRace(ctx, releaseID, by)
	}
	return err
}

// settleHeldMany holds a release for several Slots until its wait is over.
func (s *Searcher) settleHeldMany(ctx context.Context, slotIDs []int64, releaseID int64, name string, until time.Time, by domain.Actor) error {
	if len(slotIDs) == 1 {
		return s.settleHeld(ctx, slotIDs[0], releaseID, name, until, by)
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range slotIDs {
			from, err := choiceStateTx(tx, id)
			if err != nil {
				return err
			}
			if !slices.Contains(pickStates, from) {
				return busyErr(from)
			}
		}
		for _, id := range slotIDs {
			if err := passOverHeldTx(tx, id, releaseID, by); err != nil {
				return err
			}
			if err := moveSlotTx(tx, id, mediafiles.SlotHeld, "holding "+name, by, releaseID); err != nil {
				return err
			}
			if err := mediafiles.SetSlotHeldTx(tx, id, releaseID, until); err != nil {
				return err
			}
		}
		return pickReleaseTx(tx, releaseID, "held", &until, slotIDs, by)
	})
	if errors.Is(err, ErrBusy) {
		s.lostRace(ctx, releaseID, by)
	}
	return err
}

// pickReleaseTx marks a release chosen or held for its Slots. The Slots it
// fills are added to the verdict's pick_slot_ids, which the Download reads;
// Slots of another Version picked for the same release stay listed, and the
// Download keeps to one Version and the Slots still chosen for it.
func pickReleaseTx(tx *sql.Tx, releaseID int64, state string, until *time.Time, slotIDs []int64, by domain.Actor) error {
	return markPickedTx(tx, releaseID, state, until, slotIDs, by)
}

// PickSlots lists the Slots a release was chosen or held for when it fills
// more than one, or nil otherwise.
func PickSlots(rel domain.Release) []int64 {
	xs, _ := rel.Verdict["pick_slot_ids"].([]any)
	if len(xs) < 2 {
		return nil
	}
	out := make([]int64, 0, len(xs))
	for _, x := range xs {
		if f, ok := x.(float64); ok {
			out = append(out, int64(f))
		}
	}
	return out
}
