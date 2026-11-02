package fetchq

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A priority whose due jobs are all held back (Hold) never starves the
// lower priorities: the claim skips it and takes the next priority's job.
func TestHeldPriorityDoesNotStarveOthers(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{Name: "t", Rate: 100, Now: c.now, Logf: func(string, ...any) {},
		Do:   func(context.Context, string, int) error { return nil },
		Hold: func(key string, _ int) (bool, time.Duration) { return strings.HasPrefix(key, "v:"), time.Second }})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx := context.Background()
	for i := range 120 {
		q.Enqueue(ctx, fmt.Sprintf("v:%d", i), P1Finish, false)
	}
	q.Enqueue(ctx, "img:x", P2Shared, false)
	key, _, p, ok := q.claim(ctx, P4Deep)
	if !ok || key != "img:x" || p != P2Shared {
		t.Fatalf("claimed %q P%d %v", key, p, ok)
	}
}
