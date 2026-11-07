package main

import (
	"context"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbdump"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
)

// The "disk" block of the status answer, shown in HolyDash: the volume's
// total, used and free bytes, and the part this server holds (its data
// folder plus the slim MusicBrainz copies), so the rest is other programs
// on the VM. The server's part walks the folders, so it is counted in the
// background at most every diskShareEvery and the last count is answered.

const diskShareEvery = 10 * time.Minute

type diskShare struct {
	dirs  []string
	now   func() time.Time
	count func(dir string) (int64, error)

	mu      sync.Mutex
	bytes   int64
	at      time.Time
	running bool
}

// own answers the last count of the server's bytes and when it was made
// (zero before the first), starting a new count when it is stale.
func (d *diskShare) own() (int64, time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running && d.now().Sub(d.at) >= diskShareEvery {
		d.running = true
		go d.recount()
	}
	return d.bytes, d.at
}

func (d *diskShare) recount() {
	var n int64
	for _, dir := range d.dirs {
		if b, err := d.count(dir); err == nil {
			n += b
		}
	}
	d.mu.Lock()
	d.bytes, d.at, d.running = n, d.now(), false
	d.mu.Unlock()
}

// openDiskShare adds the "disk" block; dirs left "" are skipped.
func openDiskShare(dataDir string, dirs []string, o *api.Options) {
	d := &diskShare{now: time.Now, count: mbdump.DirBytes}
	for _, dir := range dirs {
		if dir != "" {
			d.dirs = append(d.dirs, dir)
		}
	}
	prev := o.StatusExtra
	o.StatusExtra = func(ctx context.Context, window time.Duration) map[string]any {
		m := map[string]any{}
		if prev != nil {
			m = prev(ctx, window)
		}
		used, total, err := refresh.DiskUsage(dataDir)
		if err != nil || total == 0 {
			return m
		}
		block := map[string]any{"total_bytes": total, "used_bytes": used, "free_bytes": total - used}
		if own, at := d.own(); !at.IsZero() {
			block["metadata_bytes"] = own
			block["metadata_counted_at"] = at.UTC()
		}
		m["disk"] = block
		return m
	}
}
