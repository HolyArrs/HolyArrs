package stats

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A write that fails (here a trigger refuses every bucket write) never
// turns the statistics into a 503: the figures not written yet come from
// memory, the cause is shown as flush_error and logged once a minute, and
// the next working write clears it.
func TestSnapshotSurvivesAFailedWrite(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var logs []string
	r, err := Open(filepath.Join(t.TempDir(), "s.sqlite"), Options{Now: func() time.Time { return now }, Logf: func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Request(200, "hit", 5*time.Millisecond)
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`CREATE TRIGGER no_writes BEFORE UPDATE ON buckets BEGIN SELECT RAISE(FAIL, 'disk said no'); END`); err != nil {
		t.Fatal(err)
	}
	r.Request(200, "miss", 5*time.Millisecond)
	r.Request(500, "", 5*time.Millisecond)
	snap, err := r.Snapshot(time.Hour)
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if snap.Totals.Requests != 3 || snap.Totals.Failures != 1 || len(snap.Series) != 1 {
		t.Fatalf("totals %+v series %d", snap.Totals, len(snap.Series))
	}
	if !strings.Contains(snap.FlushError, "disk said no") || len(logs) != 1 || !strings.Contains(logs[0], "disk said no") {
		t.Fatalf("cause not shown: %q %v", snap.FlushError, logs)
	}
	// Logged once a minute, not on every ask.
	r.Snapshot(time.Hour)
	if len(logs) != 1 {
		t.Fatalf("logged %d times", len(logs))
	}
	if _, err := r.db.Exec(`DROP TRIGGER no_writes`); err != nil {
		t.Fatal(err)
	}
	snap, err = r.Snapshot(time.Hour)
	if err != nil || snap.FlushError != "" || snap.Totals.Requests != 3 {
		t.Fatalf("after recovery: %v %q %+v", err, snap.FlushError, snap.Totals)
	}
}
