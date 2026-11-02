package imgstore

import (
	"context"
	"testing"
	"time"
)

func TestTraceAddsPhases(t *testing.T) {
	ctx, tr := WithTrace(context.Background())
	if TraceOf(ctx) != tr {
		t.Fatal("trace not carried by ctx")
	}
	addPhase(ctx, PhaseChain, 2*time.Millisecond)
	addPhase(ctx, PhaseChain, 3*time.Millisecond)
	addPhase(ctx, PhaseBig, time.Millisecond)
	if tr.D[PhaseChain] != 5*time.Millisecond || tr.D[PhaseBig] != time.Millisecond {
		t.Fatalf("got %v", tr.D)
	}
	addPhase(context.Background(), PhaseChain, time.Second) // no trace: no panic
}
