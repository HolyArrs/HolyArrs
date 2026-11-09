package downloads

import (
	"context"
	"testing"
)

// Status must exist after a poll and be gone after Drop.
func TestCursors_StatusAfterPoll(t *testing.T) {
	var c Cursors
	if _, ok := c.Status(1); ok {
		t.Fatal("status before any read")
	}
	if _, err := c.Poll(context.Background(), 1, 0, &fakeMain{answers: []string{full1}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Status(1); !ok {
		t.Fatal("no status after a read")
	}
	c.Drop(1)
	if _, ok := c.Status(1); ok {
		t.Fatal("status kept after drop")
	}
}
