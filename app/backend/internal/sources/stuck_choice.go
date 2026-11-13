package sources

import (
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// ChoiceWait is how long a chosen Slot waits for a Downloader to take its
// Release before a person may choose another one for it.
var ChoiceWait = 10 * time.Minute

// StuckChoiceReason is shown on a Slot whose chosen Release no Downloader has
// confirmed after ChoiceWait.
const StuckChoiceReason = "No Downloader has confirmed this Release yet. You can choose another one."

// StuckChoice reports whether a chosen Slot is still waiting for a Downloader
// after ChoiceWait: nothing is downloading for it.
func StuckChoice(sl mediafiles.Slot, now time.Time) bool {
	return sl.State == mediafiles.SlotChosen && sl.ActiveDownloadID == nil && !sl.UpdatedAt.IsZero() && now.Sub(sl.UpdatedAt) >= ChoiceWait
}
