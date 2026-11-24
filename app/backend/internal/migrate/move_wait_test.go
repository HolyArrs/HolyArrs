package migrate

import (
	"testing"
	"time"
)

func TestMoveWaitNote(t *testing.T) {
	for _, c := range []struct {
		done, total, left int
		next              time.Duration
		want              string
	}{
		{120, 1550, 1, 10 * time.Minute, "120 of 1550 imported wait 600 1"},
		{0, 3, 1, 1500 * time.Millisecond, "0 of 3 imported wait 2 1"},
		{3, 3, 1, 0, "3 of 3 imported wait 0 1"},
	} {
		if got := MoveWaitNote(c.done, c.total, c.next, c.left); got != c.want {
			t.Fatalf("MoveWaitNote = %q, want %q", got, c.want)
		}
	}
}
