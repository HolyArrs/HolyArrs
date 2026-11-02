package imgstore

import (
	"context"
	"time"
)

// Phases of one picture fetch, timed when the ctx carries a Trace.
const (
	PhaseChain    = iota // source lookups (search, ids)
	PhaseDownload        // picture downloads
	PhaseSlotWait        // waiting for a CPU slot
	PhaseWork            // decode, resize, encode, write
	PhaseBig             // big size made inline
	NumPhases
)

// PhaseNames names the phases for logs.
var PhaseNames = [NumPhases]string{"chain", "dl", "slotwait", "work", "big"}

// Trace sums the time one fetch spends per phase.
type Trace struct{ D [NumPhases]time.Duration }

type traceKey struct{}

// WithTrace attaches a new Trace to ctx.
func WithTrace(ctx context.Context) (context.Context, *Trace) {
	t := &Trace{}
	return context.WithValue(ctx, traceKey{}, t), t
}

// TraceOf reads the Trace on ctx (nil without one).
func TraceOf(ctx context.Context) *Trace { t, _ := ctx.Value(traceKey{}).(*Trace); return t }

func addPhase(ctx context.Context, p int, d time.Duration) {
	if t := TraceOf(ctx); t != nil {
		t.D[p] += d
	}
}

func since(ctx context.Context, p int, t0 time.Time) { addPhase(ctx, p, time.Since(t0)) }
