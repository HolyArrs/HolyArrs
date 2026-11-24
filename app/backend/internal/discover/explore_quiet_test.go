package discover

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The hold logs once when a spell starts and once when it ends; a browser
// that left (context canceled) is not an outage.
func TestServerHoldLogsStateChangesOnly(t *testing.T) {
	down := metadata.ErrServiceUnavailable
	canceled := fmt.Errorf("%w: %w", metadata.ErrServiceUnavailable, context.Canceled)
	cases := []struct {
		name  string
		steps []error // nil = an answer
		held  int     // held() calls answered true mid-spell
		lines []string
	}{
		{"answers only", []error{nil, nil, nil}, 0, nil},
		{"canceled only", []error{canceled, canceled, nil}, 0, nil},
		{"one spell", []error{down, down, down, nil, nil}, 3, []string{"not answering", "answers again after 3s, 3 list asks waited"}},
		{"canceled mid spell", []error{down, canceled, nil}, 0, []string{"not answering", "answers again after 2s, 0 list asks waited"}},
		{"bare canceled", []error{context.Canceled, nil}, 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := captureLog(t)
			var h serverHold
			now := time.Unix(1000, 0)
			for i, err := range c.steps {
				if i == 1 {
					for range c.held {
						h.held(now)
					}
				}
				h.note(err, now)
				now = now.Add(time.Second)
			}
			got := strings.Split(strings.TrimSpace(b.String()), "\n")
			if b.Len() == 0 {
				got = nil
			}
			if len(got) != len(c.lines) {
				t.Fatalf("lines = %q, want %d", got, len(c.lines))
			}
			for i, w := range c.lines {
				if !strings.Contains(got[i], w) {
					t.Fatalf("line %d = %q, want %q", i, got[i], w)
				}
			}
		})
	}
	_ = errors.New
}

func TestPoolLineQuietWhenFastAndLocal(t *testing.T) {
	cases := []struct {
		took  time.Duration
		calls int
		want  bool
	}{
		{0, 0, false},
		{900 * time.Millisecond, 0, false},
		{2 * time.Second, 0, true},
		{0, 3, true},
	}
	for _, c := range cases {
		if got := poolLineDue(c.took, c.calls); got != c.want {
			t.Fatalf("poolLineDue(%s, %d) = %v", c.took, c.calls, got)
		}
	}
}

func TestPageViewKeepsEmptyRetryBackoff(t *testing.T) {
	e := &Explore{}
	k := PoolKey{Member: 1, Switch: "movies"}
	now := time.Unix(1000, 0)
	e.noteEmpty(k, true, now)
	want := e.due[k]
	e.QueueSeen(k, now)
	if !e.due[k].Equal(want) {
		t.Fatalf("page view moved the empty retry from %s to %s", want, e.due[k])
	}
	other := PoolKey{Member: 2, Switch: "movies"}
	e.QueueSeen(other, now)
	if !e.due[other].Equal(now) {
		t.Fatalf("a missing pool is not queued now")
	}
}

func TestPageViewSkipsEmptyPoolWithNoRetryQueued(t *testing.T) {
	cases := []struct {
		name  string
		tries int
	}{
		{"retries used up", emptyTriesMax},
		{"build running, retry not queued yet", 0},
	}
	for _, c := range cases {
		e := &Explore{}
		k := PoolKey{Member: 1, Switch: "movies"}
		e.empty = map[PoolKey]int{k: c.tries}
		e.QueueSeen(k, time.Unix(1000, 0))
		if _, ok := e.due[k]; ok {
			t.Fatalf("%s: page view queued a rebuild of an empty pool", c.name)
		}
	}
}
