package fetchq

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readyPriorities finds each priority with a job due by index seeks, not
// a scan of every queued row: a priority whose jobs all wait for later is
// not ready.
func TestReadyPriorities(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error { return nil })
	ctx := context.Background()
	q.Enqueue(ctx, "a", P1Finish, false)
	q.Enqueue(ctx, "b", P3Refresh, false)
	q.Enqueue(ctx, "c", P4Deep, false)
	if _, err := q.db.Exec(`UPDATE fetch_jobs SET not_before=? WHERE key='c'`, c.t.Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	got := q.readyPriorities(ctx, c.t, P4Deep)
	want := [5]bool{false, true, false, true, false}
	if got != want {
		t.Fatalf("ready %v, want %v", got, want)
	}
	if got := q.readyPriorities(ctx, c.t, P1Finish); got != [5]bool{false, true} {
		t.Fatalf("capped at P1: %v", got)
	}
	var plan string
	rows, err := q.db.Query(`EXPLAIN QUERY PLAN `+readySQL, P1Finish, c.t.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b, d int
		var s string
		_ = rows.Scan(&a, &b, &d, &s)
		plan += s + ";"
	}
	rows.Close()
	if want := "USING COVERING INDEX fetch_jobs_pick (state=? AND priority=? AND not_before<?)"; !strings.Contains(plan, want) {
		t.Fatalf("plan %q lacks %q", plan, want)
	}
}
