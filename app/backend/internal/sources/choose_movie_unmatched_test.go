package sources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestChoose_MovieUnmatchedNameForcedGoesToSearchedSlot checks that "Pick anyway" on a movie Release whose name matched no Slot goes to the Slot that was searched, not an episode error.
func TestChoose_MovieUnmatchedNameForcedGoesToSearchedSlot(t *testing.T) {
	const odd = "[ToonsHub] Some Other Name 2024 1080p WEB-DL H.264"
	lf := &liveFeed{}
	lf.set(feedOf(item(odd, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"choose": false})
	rel := relByName(t, f, odd)
	if len(rel.MatchedSlotIDs) != 0 {
		t.Fatalf("release matched %v; the case needs an unmatched name", rel.MatchedSlotIDs)
	}
	sr := &Searcher{Store: f.s}
	ctx := context.Background()
	var nf *NeedsForceError
	if _, err := sr.Choose(ctx, rel.ID, 0, false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("without force: %v, want needs force", err)
	}
	ch, err := sr.Choose(ctx, rel.ID, 0, true, domain.System)
	if err != nil {
		t.Fatalf("forced pick: %v", err)
	}
	if ch.SlotID != f.slotID {
		t.Fatalf("went to slot %d, want %d", ch.SlotID, f.slotID)
	}
}
