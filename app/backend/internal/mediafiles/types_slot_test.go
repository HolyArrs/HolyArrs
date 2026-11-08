package mediafiles

import "testing"

func TestSlotTransitionsAllowed(t *testing.T) {
	// Every arrow in model.md section 4.
	moves := [][2]SlotState{
		{SlotNone, SlotNotOutYet}, {SlotNone, SlotWanted},
		{SlotNotOutYet, SlotWanted},
		{SlotWanted, SlotSearching}, {SlotWanted, SlotNotOutYet},
		{SlotSearching, SlotChosen}, {SlotSearching, SlotHeld}, {SlotSearching, SlotWanted},
		{SlotHeld, SlotChosen},
		{SlotChosen, SlotDownloading}, {SlotChosen, SlotWanted}, {SlotChosen, SlotNeedsADecision},
		{SlotDownloading, SlotStalled}, {SlotDownloading, SlotFailed}, {SlotDownloading, SlotImporting},
		{SlotStalled, SlotDownloading}, {SlotStalled, SlotSearching}, {SlotStalled, SlotFailed}, {SlotSearching, SlotNeedsADecision},
		{SlotFailed, SlotSearching}, {SlotFailed, SlotNeedsADecision},
		{SlotImporting, SlotNeedsADecision}, {SlotImporting, SlotRejected}, {SlotImporting, SlotInLibrary},
		{SlotNeedsADecision, SlotImporting}, {SlotNeedsADecision, SlotWanted},
		{SlotRejected, SlotSearching},
		{SlotInLibrary, SlotBetterWanted}, {SlotInLibrary, SlotWanted}, {SlotInLibrary, SlotRemoved},
		{SlotBetterWanted, SlotSearching}, {SlotBetterWanted, SlotInLibrary},
		// Stage 3.1: hand search past the gate, back to where a search
		// started, Held removed by hand, a hand choice over a File.
		{SlotNotOutYet, SlotSearching},
		{SlotSearching, SlotBetterWanted}, {SlotSearching, SlotNotOutYet},
		{SlotHeld, SlotSearching}, {SlotHeld, SlotWanted}, {SlotHeld, SlotBetterWanted},
		{SlotInLibrary, SlotSearching},
		// Stage 4.1: removed by hand without a new search; a person decides.
		{SlotFailed, SlotWanted}, {SlotFailed, SlotBetterWanted},
		{SlotNeedsADecision, SlotSearching}, {SlotNeedsADecision, SlotChosen}, {SlotNeedsADecision, SlotBetterWanted},
		// Stage 4.2.1: a pack that did not hold an episode.
		{SlotImporting, SlotWanted}, {SlotImporting, SlotBetterWanted},
	}
	allowed := map[[2]SlotState]bool{}
	for _, m := range moves {
		allowed[m] = true
		if err := Transition(m[0], m[1]); err != nil {
			t.Errorf("%s -> %s should be allowed: %v", m[0], m[1], err)
		}
	}
	// Every other pair is refused.
	from := append([]SlotState{SlotNone}, SlotStates()...)
	for _, a := range from {
		for _, b := range SlotStates() {
			if allowed[[2]SlotState{a, b}] {
				continue
			}
			if err := Transition(a, b); err == nil {
				t.Errorf("%q -> %q should be refused", a, b)
			}
		}
	}
}

func TestSlotTransitionUnknownState(t *testing.T) {
	if err := Transition(SlotWanted, "nope"); err == nil {
		t.Fatal("unknown target accepted")
	}
	if err := Transition("nope", SlotWanted); err == nil {
		t.Fatal("unknown source accepted")
	}
	if SlotState("").Valid() {
		t.Fatal("empty state should not be valid")
	}
}

func TestIllegalExamples(t *testing.T) {
	for _, m := range [][2]SlotState{
		{SlotWanted, SlotInLibrary},
		{SlotNotOutYet, SlotDownloading},
		{SlotRemoved, SlotWanted},
		{SlotHeld, SlotDownloading},
		{SlotNotOutYet, SlotInLibrary},
	} {
		if err := Transition(m[0], m[1]); err == nil {
			t.Errorf("%s -> %s should be refused", m[0], m[1])
		}
	}
}
