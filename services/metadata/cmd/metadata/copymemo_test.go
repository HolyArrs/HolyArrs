package main

import (
	"context"
	"testing"
)

// An answer is kept for its copy only; with no copy, or a read cut short,
// it is not kept; past the limit the memo starts over.
func TestCopyMemo(t *testing.T) {
	ver, reads := "", 0
	m := newCopyMemo[int](func() string { return ver }, 2)
	read := func() int { reads++; return reads }
	ctx := context.Background()
	m.get(ctx, "a", read)
	m.get(ctx, "a", read)
	if reads != 2 {
		t.Fatalf("no copy: kept (reads %d)", reads)
	}
	ver = "d1"
	if v := m.get(ctx, "a", read); v != 3 || m.get(ctx, "a", read) != 3 {
		t.Fatal("not kept for its copy")
	}
	ver = "d2"
	if m.get(ctx, "a", read) != 4 {
		t.Fatal("kept across a new copy")
	}
	cut, cancel := context.WithCancel(ctx)
	cancel()
	m.get(cut, "b", read)
	if m.get(ctx, "b", read) != 6 {
		t.Fatal("a read cut short was kept")
	}
	m.get(ctx, "c", read) // a, b full: starts over, then c
	if m.get(ctx, "a", read) != 8 {
		t.Fatal("limit not kept")
	}
}
