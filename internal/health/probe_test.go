package health

import (
	"context"
	"testing"
)

func TestProbeStableHealthIsReadOnly(t *testing.T) {
	w := newWorld(t)
	w.run(t)
	writes := w.s.WriteCount()
	needed, _, err := w.e.Probe(context.Background(), w.now)
	if err != nil || needed {
		t.Fatalf("stable checks: needed %v, error %v", needed, err)
	}
	if got := w.s.WriteCount(); got != writes {
		t.Fatalf("probe wrote %d transactions", got-writes)
	}

	w.free = 500 << 20
	needed, _, err = w.e.Probe(context.Background(), w.now)
	if err != nil || !needed {
		t.Fatalf("changed check: needed %v, error %v", needed, err)
	}
	if got := w.s.WriteCount(); got != writes {
		t.Fatalf("changed probe wrote %d transactions", got-writes)
	}
}

func TestProbeKeepsUnconfirmedFlapMoving(t *testing.T) {
	w := newWorld(t)
	w.run(t)
	w.free = 500 << 20
	w.run(t) // first warning; guard is two runs, so it is not confirmed yet
	writes := w.s.WriteCount()
	needed, _, err := w.e.Probe(context.Background(), w.now)
	if err != nil || !needed {
		t.Fatalf("unconfirmed check: needed %v, error %v", needed, err)
	}
	if got := w.s.WriteCount(); got != writes {
		t.Fatalf("flap probe wrote %d transactions", got-writes)
	}
}
