package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Pins of the lookup_cache reads and writes (the table moves into metadata):
// a large table, the expiry edge, replace on the same key, and the purge of
// at most 100 expired rows per write.
func TestPinLookupCacheLargeTable(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c := LookupCache{Store: s}
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	// 1,500 expired rows and 1,200 live ones.
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for i := 0; i < 1500; i++ {
			if _, err := tx.Exec("INSERT INTO lookup_cache (key, body, fetched_at, expires_at) VALUES (?, ?, ?, ?)",
				fmt.Sprintf("old:%d", i), []byte("x"), database.FormatTime(now.Add(-48*time.Hour)), database.FormatTime(now.Add(-time.Hour))); err != nil {
				return err
			}
		}
		for i := 0; i < 1200; i++ {
			if _, err := tx.Exec("INSERT INTO lookup_cache (key, body, fetched_at, expires_at) VALUES (?, ?, ?, ?)",
				fmt.Sprintf("live:%d", i), []byte(fmt.Sprintf("body%d", i)), database.FormatTime(now), database.FormatTime(now.Add(time.Hour))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if b, ok, err := c.Get(ctx, "live:777", now); err != nil || !ok || string(b) != "body777" {
		t.Fatalf("live get: %q %v %v", b, ok, err)
	}
	if _, ok, _ := c.Get(ctx, "old:5", now); ok {
		t.Fatal("expired row served")
	}
	if _, ok, _ := c.Get(ctx, "missing", now); ok {
		t.Fatal("missing row served")
	}
	// The expiry instant itself is already expired (expires_at > now).
	if _, ok, _ := c.Get(ctx, "live:1", now.Add(time.Hour)); ok {
		t.Fatal("row served at its expiry instant")
	}
	count := func(where string) int {
		var n int
		if err := s.Read().QueryRow("SELECT count(*) FROM lookup_cache WHERE " + where).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := c.Put(ctx, "new", []byte("v1"), now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// One write drops at most 100 expired rows and keeps every live one.
	if n := count("key LIKE 'old:%'"); n != 1400 {
		t.Fatalf("expired left %d, want 1400", n)
	}
	if n := count("key LIKE 'live:%'"); n != 1200 {
		t.Fatalf("live left %d", n)
	}
	// The same key is replaced, not added.
	if err := c.Put(ctx, "new", []byte("v2"), now, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if b, ok, _ := c.Get(ctx, "new", now.Add(90*time.Minute)); !ok || string(b) != "v2" {
		t.Fatalf("replaced: %q %v", b, ok)
	}
	if n := count("key = 'new'"); n != 1 {
		t.Fatalf("new rows %d", n)
	}
}
