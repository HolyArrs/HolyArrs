package discover

import "testing"

// TestMemoReadOnce: a memo reads each id once, misses included; no memo reads every time.
func TestMemoReadOnce(t *testing.T) {
	t.Parallel()
	calls := 0
	get := func(v string, ok bool) func() (string, bool) {
		return func() (string, bool) { calls++; return v, ok }
	}
	seen := map[int64]*string{}
	for range 3 {
		if v, ok := memoRead(seen, 1, get("Movies", true)); v != "Movies" || !ok {
			t.Fatalf("got %q %v", v, ok)
		}
		if v, ok := memoRead(seen, 2, get("", false)); v != "" || ok {
			t.Fatalf("missing got %q %v", v, ok)
		}
	}
	if calls != 2 {
		t.Fatalf("reads %d, want 2", calls)
	}
	calls = 0
	for range 3 {
		memoRead(nil, 1, get("Movies", true))
	}
	if calls != 3 {
		t.Fatalf("no memo reads %d, want 3", calls)
	}
}
