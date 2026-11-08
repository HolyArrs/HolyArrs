package metadata

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

type keptThing struct{ N int }

func TestKeepOnDiskRoundTrip(t *testing.T) {
	st, _ := openTest(t, database.Options{})
	ctx := context.Background()
	KeepOnDisk(ctx, st, "title|movie|603", keptThing{7})
	k, ok := LoadKept[keptThing](ctx, st, "title|movie|603")
	if !ok || k.V.N != 7 || time.Since(k.At) > time.Minute {
		t.Fatalf("round trip: %+v %v", k, ok)
	}
	if _, ok := LoadKept[keptThing](ctx, st, "title|movie|missing"); ok {
		t.Fatal("missing key answered")
	}
	if _, ok := LoadKept[keptThing](ctx, nil, "title|movie|603"); ok {
		t.Fatal("nil store answered")
	}
	KeepOnDisk(ctx, nil, "title|movie|1", keptThing{1}) // must not panic
}

func TestKeepOnDiskCapsPerPrefix(t *testing.T) {
	old := KeptPerPrefix
	KeptPerPrefix = 3
	t.Cleanup(func() { KeptPerPrefix = old })
	st, _ := openTest(t, database.Options{})
	ctx := context.Background()
	for _, k := range []string{"a|1", "a|2", "a|3", "a|4", "a|5"} {
		KeepOnDisk(ctx, st, k, keptThing{1})
		time.Sleep(2 * time.Millisecond)
	}
	KeepOnDisk(ctx, st, "b|1", keptThing{1})
	for _, k := range []string{"a|5", "a|4", "a|3", "b|1"} {
		if _, ok := LoadKept[keptThing](ctx, st, k); !ok {
			t.Errorf("%s was dropped", k)
		}
	}
	for _, k := range []string{"a|1", "a|2"} {
		if _, ok := LoadKept[keptThing](ctx, st, k); ok {
			t.Errorf("%s kept past the cap", k)
		}
	}
}

func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}

func TestRefreshOnceDedupsAndReleases(t *testing.T) {
	block := make(chan struct{})
	var n atomic.Int32
	RefreshOnce("kept-test-k", func(context.Context) { n.Add(1); <-block })
	RefreshOnce("kept-test-k", func(context.Context) { n.Add(1); <-block })
	waitUntil(t, func() bool { return n.Load() == 1 })
	// RefreshOnce decides synchronously, so the second call has already been refused.
	if n.Load() != 1 {
		t.Fatalf("second refresh ran while the first was busy: %d", n.Load())
	}
	close(block)
	waitUntil(t, func() bool { _, busy := refreshing.Load("kept-test-k"); return !busy })
	RefreshOnce("kept-test-k", func(context.Context) { n.Add(1) })
	waitUntil(t, func() bool { return n.Load() == 2 })
}

func TestDigitsOnly(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{{"603", true}, {"1", true}, {"", false}, {"0", false}, {"0603", false}, {"12a", false},
		{"1234567890123", false}, {"123456789012", true}, {"-5", false}} {
		if got := DigitsOnly(c.in); got != c.want {
			t.Errorf("DigitsOnly(%q)=%v want %v", c.in, got, c.want)
		}
	}
}
