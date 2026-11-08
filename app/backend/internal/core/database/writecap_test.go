package database

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

// A write past the cap is rolled back and the writer moves on to the next.
func TestWriteCapRollsBackAndMovesOn(t *testing.T) {
	s, _ := openTest(t, Options{WriteCap: 100 * time.Millisecond})
	ctx := context.Background()
	release := make(chan struct{})
	defer close(release)
	began := time.Now()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO settings(created_at, created_by, updated_at, updated_by, grp, key, value) VALUES ('t', '{}', 't', '{}', 'this_app', 'held_t', 'x')"); err != nil {
			return err
		}
		<-release // held up on purpose
		return nil
	})
	if !errors.Is(err, ErrWriteTooLong) {
		t.Fatalf("held write: %v", err)
	}
	if d := time.Since(began); d > 2*time.Second {
		t.Fatalf("cap took %v", d)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO settings(created_at, created_by, updated_at, updated_by, grp, key, value) VALUES ('t', '{}', 't', '{}', 'this_app', 'next_t', 'y')")
		return err
	}); err != nil {
		t.Fatalf("next write: %v", err)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM settings WHERE grp = 'this_app' AND key = 'held_t'").Scan(&n)
	if n != 0 {
		t.Fatal("the held write was committed")
	}
}

// Core writes go ahead of every write still waiting.
func TestCoreLaneGoesFirst(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	hold, holding := make(chan struct{}), make(chan struct{})
	go func() {
		_ = s.Write(ctx, func(*sql.Tx) error { close(holding); <-hold; return nil })
	}()
	<-holding
	var mu sync.Mutex
	var order []string
	var wg sync.WaitGroup
	write := func(name string, core bool) {
		defer wg.Done()
		fn := func(*sql.Tx) error { mu.Lock(); order = append(order, name); mu.Unlock(); return nil }
		if core {
			_ = s.WriteCore(ctx, fn)
		} else {
			_ = s.Write(ctx, fn)
		}
	}
	wg.Add(4)
	for _, n := range []string{"a", "b", "c"} {
		go write(n, false)
	}
	time.Sleep(50 * time.Millisecond)
	go write("core", true)
	time.Sleep(50 * time.Millisecond)
	close(hold)
	wg.Wait()
	if len(order) != 4 || order[0] != "core" {
		t.Fatalf("order %v", order)
	}
}

// Migrations and nested writes still work with the function on its own goroutine.
func TestNestedWriteStillCaughtOnCoreLane(t *testing.T) {
	s, _ := openTest(t, Options{})
	var inner error
	if err := s.WriteCore(context.Background(), func(*sql.Tx) error {
		inner = s.Write(context.Background(), func(*sql.Tx) error { return nil })
		return nil
	}); err != nil || !errors.Is(inner, ErrNestedWrite) {
		t.Fatalf("outer %v inner %v", err, inner)
	}
}
