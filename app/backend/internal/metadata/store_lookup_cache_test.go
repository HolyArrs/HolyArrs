package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TestLookupCacheExpiry: a body is answered until its expiry, replaced by
// a newer Put, and expired rows are dropped by later Puts.
func TestLookupCacheExpiry(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	c := LookupCache{Store: s}
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	if _, ok, err := c.Get(ctx, "k", now); ok || err != nil {
		t.Fatalf("empty cache: %v %v", ok, err)
	}
	if err := c.Put(ctx, "k", []byte("one"), now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if b, ok, err := c.Get(ctx, "k", now.Add(59*time.Minute)); !ok || err != nil || string(b) != "one" {
		t.Fatalf("hit: %q %v %v", b, ok, err)
	}
	if _, ok, _ := c.Get(ctx, "k", now.Add(time.Hour)); ok {
		t.Fatal("answered at its expiry")
	}
	if err := c.Put(ctx, "k", []byte("two"), now, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if b, ok, _ := c.Get(ctx, "k", now.Add(90*time.Minute)); !ok || string(b) != "two" {
		t.Fatalf("replaced: %q %v", b, ok)
	}
	if err := c.Put(ctx, "other", []byte("x"), now.Add(3*time.Hour), now.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM lookup_cache").Scan(&n); err != nil || n != 1 {
		t.Fatalf("expired rows left: %d %v", n, err)
	}
}

func openTest(t *testing.T, opts database.Options) (*database.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}
