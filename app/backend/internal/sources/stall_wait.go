package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// holdStalled holds back a passing Release whose Downloads stalled in a row,
// for a wait that grows with each stall (mediafiles.StallWait). A dead VPN
// then does not get the same Release sent into it every day. The Release
// is never turned down for it, and a person's own search is not held.
func (s *Searcher) holdStalled(ctx context.Context, rels []mediafiles.ReleaseFacts, vs []mediafiles.Verdict, hand bool, now time.Time) {
	if hand {
		return
	}
	var names []string
	for i := range vs {
		if vs[i].Passed {
			names = append(names, rels[i].Name)
		}
	}
	stalls, err := mediafiles.StallStreaks(ctx, s.Store.Read(), names)
	if err != nil || len(stalls) == 0 {
		return
	}
	for i := range vs {
		st, ok := stalls[rels[i].Name]
		until := st.Until()
		if !ok || !vs[i].Passed || !now.Before(until) || vs[i].HoldUntil != nil && !vs[i].HoldUntil.Before(until) {
			continue
		}
		vs[i].Outcome, vs[i].HoldUntil = "held", &until
		vs[i].Reasons = append(vs[i].Reasons, mediafiles.Reason{Step: "wait", Rule: "stalled", Effect: "hold",
			Detail: fmt.Sprintf("held until %s: its download stalled %d times in a row", until.UTC().Format(time.RFC3339), st.Count)})
	}
}
