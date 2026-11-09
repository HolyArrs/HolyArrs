package music

import (
	"errors"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestGapOfPassesQueueWordsThrough(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("x: %w", &metadata.PendingError{State: "queued", Position: 3, Message: "Queued, number 3 in line"})
	g := gapOf(err)
	if g == nil || g.State != "queued" || g.Position != 3 || g.Message != "Queued, number 3 in line" {
		t.Fatalf("got %#v", g)
	}
	if g := gapOf(metadata.ErrServicePending); g == nil || g.State != "fetching" || g.Message == "" {
		t.Fatalf("bare pending: %#v", g)
	}
	if gapOf(errors.New("boom")) != nil || gapOf(nil) != nil {
		t.Fatal("other errors are not a gap")
	}
}
