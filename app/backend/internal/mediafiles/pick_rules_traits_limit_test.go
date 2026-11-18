package mediafiles

import (
	"strings"
	"testing"
)

// Traits, must-have and never lists take up to 256 entries (pushed and
// synced formats number in the hundreds); steps, tags and size limits
// keep their smaller limit.
func TestCheckPickRules_TraitListLimit(t *testing.T) {
	ts := make([]OwnTrait, 0, 300)
	for i := 1; i <= 300; i++ {
		ts = append(ts, OwnTrait{Name: "T", Match: "name"})
		ts[i-1].ID = int64(i)
	}
	SetOwnTraits(ts)
	t.Cleanup(func() { SetOwnTraits(nil) })
	video := []string{"movie", "show"}
	for _, n := range []int{256, 257} {
		b := GoodEveryday()
		b.Traits, b.MustHave, b.Never = nil, nil, nil
		for i := 1; i <= n; i++ {
			b.Traits = append(b.Traits, TraitPoints{Trait: OwnTraitKey(int64(i)), Points: 1})
			b.MustHave = append(b.MustHave, OwnTraitKey(int64(i)))
			b.Never = append(b.Never, OwnTraitKey(int64(i)))
		}
		err := CheckPickRules("x", video, b)
		if n == 256 && err != nil {
			t.Fatalf("256 listed: %v", err)
		}
		if n == 257 && (err == nil || !strings.Contains(err.Error(), "256")) {
			t.Fatalf("257 listed: %v", err)
		}
	}
	b := GoodEveryday()
	for i := 0; i < 65; i++ {
		b.Tags = append(b.Tags, "t")
	}
	if err := CheckPickRules("x", video, b); err == nil || !strings.Contains(err.Error(), "64 tags") {
		t.Fatalf("65 tags: %v", err)
	}
}
