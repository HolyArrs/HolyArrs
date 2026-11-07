package main

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
)

// The block has the volume's numbers at once and the server's bytes (data
// folder plus slim root) once the background count has run.
func TestDiskShareBlock(t *testing.T) {
	data, slim := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(data, "a"), make([]byte, 1000), 0o600)
	os.MkdirAll(filepath.Join(slim, "20261104-002222"), 0o755)
	os.WriteFile(filepath.Join(slim, "20261104-002222", "b"), make([]byte, 500), 0o600)
	o := &api.Options{}
	openDiskShare(data, []string{data, slim, ""}, o)
	block := func() map[string]any {
		return o.StatusExtra(context.Background(), time.Hour)["disk"].(map[string]any)
	}
	b := block()
	if b["total_bytes"].(uint64) == 0 || b["free_bytes"].(uint64) > b["total_bytes"].(uint64) {
		t.Fatalf("volume: %+v", b)
	}
	deadline := time.Now().Add(5 * time.Second)
	for b["metadata_bytes"] == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		b = block()
	}
	if b["metadata_bytes"] != int64(1500) {
		t.Fatalf("server bytes: %+v", b)
	}
}

// A failed count keeps the last good one and its time.
func TestDiskShareKeepsLastOnError(t *testing.T) {
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	d := &diskShare{dirs: []string{"a", "b"}, now: func() time.Time { return at.Add(time.Hour) }, bytes: 99, at: at,
		count: func(dir string) (int64, error) {
			if dir == "b" {
				return 0, os.ErrNotExist
			}
			return 5, nil
		}}
	d.recount()
	if b, when := d.own(); b != 99 || !when.Equal(at) {
		t.Fatalf("kept %d at %v", b, when)
	}
}

// A count is not started again while one runs or before it is stale.
func TestDiskShareCountsRarely(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).UnixNano())
	var n atomic.Int32
	calls := func() int32 { return n.Load() }
	d := &diskShare{dirs: []string{"x"}, now: func() time.Time { return time.Unix(0, clock.Load()) },
		count: func(string) (int64, error) { n.Add(1); return 7, nil }}
	d.own()
	for i := 0; i < 100; i++ {
		if b, _ := d.own(); b == 7 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	d.own()
	if b, _ := d.own(); calls() != 1 || b != 7 {
		t.Fatalf("calls %d, bytes %d", calls(), b)
	}
	clock.Add(int64(diskShareEvery))
	d.own()
	for i := 0; i < 100 && calls() == 1; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if calls() != 2 {
		t.Fatalf("stale count not redone: %d", calls())
	}
}
