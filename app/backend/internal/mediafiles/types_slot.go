package mediafiles

import "fmt"

// SlotState is where one Slot is in the core loop (model.md section 4).
type SlotState string

// Slot states.
const (
	SlotNone           SlotState = "" // before the Slot exists
	SlotNotOutYet      SlotState = "not_out_yet"
	SlotWanted         SlotState = "wanted"
	SlotSearching      SlotState = "searching"
	SlotHeld           SlotState = "held"
	SlotChosen         SlotState = "chosen"
	SlotDownloading    SlotState = "downloading"
	SlotStalled        SlotState = "stalled"
	SlotFailed         SlotState = "failed"
	SlotImporting      SlotState = "importing"
	SlotNeedsADecision SlotState = "needs_a_decision"
	SlotRejected       SlotState = "rejected"
	SlotInLibrary      SlotState = "in_library"
	SlotBetterWanted   SlotState = "better_wanted"
	SlotRemoved        SlotState = "removed" // the diagram's end state
)

var slotMoves = map[SlotState][]SlotState{
	SlotNone: {SlotNotOutYet, SlotWanted},
	// not_out_yet -> searching: a single-Item hand search may skip the gate
	// (with a warning); bulk searches never do.
	SlotNotOutYet: {SlotWanted, SlotSearching},
	// wanted -> not_out_yet: new dates or a gate change put the gate in the
	// future again (M1; not drawn in model.md section 4 yet).
	SlotWanted: {SlotSearching, SlotNotOutYet},
	// searching -> better_wanted / not_out_yet: nothing passed, so the Slot
	// goes back to where the search started (it had a File, or the gate
	// was skipped by hand).
	// searching -> needs_a_decision: a search after a failed Download found
	// nothing else that passes (stage 4.0.3).
	SlotSearching: {SlotChosen, SlotHeld, SlotWanted, SlotBetterWanted, SlotNotOutYet, SlotNeedsADecision},
	// held -> searching: a hand search or hand choice; held -> wanted or
	// better_wanted: the Held Release was removed by hand.
	SlotHeld: {SlotChosen, SlotSearching, SlotWanted, SlotBetterWanted},
	// An abandoned send with an unknown outcome needs a person's decision;
	// returning it to wanted would let automation choose it again.
	SlotChosen:      {SlotDownloading, SlotWanted, SlotNeedsADecision},
	SlotDownloading: {SlotStalled, SlotFailed, SlotImporting},
	// stalled -> failed: marked failed by hand, or acted on after the
	// act-after time (stage 4.0.4).
	SlotStalled: {SlotDownloading, SlotSearching, SlotFailed},
	// failed -> wanted / better_wanted: removed by hand without searching
	// again (stage 4.1.1).
	SlotFailed: {SlotSearching, SlotNeedsADecision, SlotWanted, SlotBetterWanted},
	// importing -> wanted / better_wanted: a pack did not hold this episode
	// (stage 4.2.1), so it looks again.
	SlotImporting: {SlotNeedsADecision, SlotRejected, SlotInLibrary, SlotWanted, SlotBetterWanted},
	// needs_a_decision -> searching / chosen / better_wanted: a person
	// searches again, chooses a Release by hand or stops looking with a
	// copy in place (stage 4.1.2).
	SlotNeedsADecision: {SlotImporting, SlotWanted, SlotSearching, SlotChosen, SlotBetterWanted},
	SlotRejected:       {SlotSearching},
	// in_library -> searching: a Release chosen by hand replaces a File
	// that is already good enough.
	SlotInLibrary: {SlotBetterWanted, SlotWanted, SlotRemoved, SlotSearching},
	// better_wanted -> in_library: the Pick rules or Keep up to date changed
	// and the current file is now enough (M1; not drawn in model.md yet).
	SlotBetterWanted: {SlotSearching, SlotInLibrary},
}

// SlotStates lists every state, for validation.
func SlotStates() []SlotState {
	return []SlotState{SlotNotOutYet, SlotWanted, SlotSearching, SlotHeld, SlotChosen,
		SlotDownloading, SlotStalled, SlotFailed, SlotImporting, SlotNeedsADecision,
		SlotRejected, SlotInLibrary, SlotBetterWanted, SlotRemoved}
}

// Valid reports whether s is a known state.
func (s SlotState) Valid() bool {
	for _, v := range SlotStates() {
		if s == v {
			return true
		}
	}
	return false
}

// Transition checks a move from one Slot state to another.
func Transition(from, to SlotState) error {
	if from != SlotNone && !from.Valid() {
		return fmt.Errorf("unknown slot state %q", from)
	}
	if !to.Valid() {
		return fmt.Errorf("unknown slot state %q", to)
	}
	for _, ok := range slotMoves[from] {
		if ok == to {
			return nil
		}
	}
	return fmt.Errorf("slot cannot move from %q to %q", from, to)
}
