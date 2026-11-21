package jobs

import (
	"context"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestLiveIDsWithKeyPrefix(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	q := New(s, Options{})
	ctx := context.Background()
	add := func(key string) int64 {
		id, _, err := q.Enqueue(ctx, "noop", nil, nil, key, database.System)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := add("youtube:7:1"), add("youtube:7:follow:2")
	add("youtube:70:3")
	add("youtube:8:4")
	done := add("youtube:7:5")
	if err := q.Cancel(ctx, done); err != nil {
		t.Fatal(err)
	}
	got, err := LiveIDsWithKeyPrefix(ctx, s.Read(), "youtube:7:")
	if err != nil || !slices.Equal(got, []int64{a, b}) {
		t.Fatalf("got %v, %v; want [%d %d]", got, err, a, b)
	}
}
