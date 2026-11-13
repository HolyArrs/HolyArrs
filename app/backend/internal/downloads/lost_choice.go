package downloads

import (
	"fmt"
	"time"
)

// GrabKey is the dedupe key of the grab Job for one choice. It names the
// Release, so a grab still waiting for an earlier choice of the same Slot (or
// of a deleted title whose Slot id was reused) never swallows a new one.
func GrabKey(slotID, releaseID int64) string {
	return fmt.Sprintf("grab:%d:%d", slotID, releaseID)
}

// lostChoiceWait is how long a chosen Slot may wait with no grab Job and no
// send on record before it is taken as lost.
const lostChoiceWait = 10 * time.Minute

// lostChoiceReason is shown on a Slot whose choice was never sent.
const lostChoiceReason = "no Downloader was asked to take it; choose it again or let a search pick one"
