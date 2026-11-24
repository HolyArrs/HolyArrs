package sources

import (
	"context"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// waitSend waits up to SendWait for the send of a pick by hand to end, and
// notes in ch whether the Downloader got it. A Slot still chosen is still
// sending; one that moved on with a "not sent" reason was refused.
func (s *Searcher) waitSend(ctx context.Context, ch *Choice) {
	if s.SendWait <= 0 || ch.SlotID == 0 {
		return
	}
	deadline := time.Now().Add(s.SendWait)
	for {
		var sl mediafiles.Slot
		if err := database.Get(ctx, s.Store.Read(), "slots", ch.SlotID, &sl); err != nil {
			return
		}
		if sl.State != mediafiles.SlotChosen {
			back := sl.State == mediafiles.SlotWanted || sl.State == mediafiles.SlotNeedsADecision
			if back && sl.LastReason != nil && strings.HasPrefix(*sl.LastReason, "not sent: ") {
				ch.NotSent = strings.TrimPrefix(*sl.LastReason, "not sent: ")
			}
			return
		}
		if !time.Now().Before(deadline) {
			ch.Sending = true
			return
		}
		select {
		case <-ctx.Done():
			ch.Sending = true
			return
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// WaitSend waits for the send of a pick by hand the way the app's Choose
// button does, for callers outside this package.
func (s *Searcher) WaitSend(ctx context.Context, ch *Choice) { s.waitSend(ctx, ch) }
