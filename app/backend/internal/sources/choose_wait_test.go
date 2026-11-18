package sources

import (
	"context"
	"testing"
	"time"
)

// Choosing by hand waits for the send, so the page never says sent for a
// Release the Downloader never got.
func TestChooseWaitsForTheSend(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	s := &Searcher{Store: f.s, SendWait: 300 * time.Millisecond}
	exec(t, f, "UPDATE slots SET state = 'needs_a_decision', chosen_release_id = NULL, last_reason = 'not sent: the Release link leads nowhere' WHERE id = ?", f.slotID)
	ch := Choice{SlotID: f.slotID, Warnings: []string{}}
	s.waitSend(ctx, &ch)
	if ch.NotSent != "the Release link leads nowhere" || ch.Sending {
		t.Fatalf("refused send: %+v", ch)
	}
	exec(t, f, "UPDATE slots SET state = 'chosen', last_reason = NULL WHERE id = ?", f.slotID)
	ch = Choice{SlotID: f.slotID, Warnings: []string{}}
	s.waitSend(ctx, &ch)
	if ch.NotSent != "" || !ch.Sending {
		t.Fatalf("send still running: %+v", ch)
	}
	exec(t, f, "UPDATE slots SET state = 'downloading' WHERE id = ?", f.slotID)
	ch = Choice{SlotID: f.slotID, Warnings: []string{}}
	s.waitSend(ctx, &ch)
	if ch.NotSent != "" || ch.Sending {
		t.Fatalf("sent: %+v", ch)
	}
	// With no wait set (the automatic paths and most tests), nothing is read.
	ch = Choice{SlotID: f.slotID, Warnings: []string{}}
	(&Searcher{Store: f.s}).waitSend(ctx, &ch)
	if ch.NotSent != "" || ch.Sending {
		t.Fatalf("no wait: %+v", ch)
	}
}
