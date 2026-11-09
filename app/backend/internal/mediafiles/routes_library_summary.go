package mediafiles

import (
	"net/url"
	"strings"
)

// StateLabels are the words people see for Slot states.
var StateLabels = map[SlotState]string{
	SlotNotOutYet: "Not out yet", SlotWanted: "Missing", SlotSearching: "Searching",
	SlotHeld: "Held", SlotChosen: "Chosen", SlotDownloading: "Downloading",
	SlotStalled: "Stalled", SlotFailed: "Failed", SlotImporting: "Importing",
	SlotNeedsADecision: "Needs a decision", SlotRejected: "Rejected", SlotInLibrary: "In library",
	SlotBetterWanted: "Better copy sought", SlotRemoved: "Removed",
}

// artworkLinks turns stored artwork ("poster:<cache name>") into links the
// web app can load (/api/v1/artwork/<cache name>); anything else is kept.
func artworkLinks(in []string) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if name, ok := strings.CutPrefix(a, "poster:"); ok && name != "" {
			a = "/api/v1/artwork/" + url.PathEscape(name)
		}
		out = append(out, a)
	}
	return out
}
