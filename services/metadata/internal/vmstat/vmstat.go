// Package vmstat records the VM's disk and memory (total, used, free) over
// time for the status page (Luis's no-growth-cap decision, 2026-10-23),
// with the 75% disk and 80% memory alerts.
package vmstat

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Sample is one reading. Memory is the host's (/proc/meminfo); Container*
// is this container's cgroup use and limit (0 when unknown).
type Sample struct {
	At             time.Time `json:"at"`
	DiskTotal      uint64    `json:"disk_total_bytes"`
	DiskUsed       uint64    `json:"disk_used_bytes"`
	DiskFree       uint64    `json:"disk_free_bytes"`
	MemTotal       uint64    `json:"mem_total_bytes"`
	MemUsed        uint64    `json:"mem_used_bytes"`
	MemFree        uint64    `json:"mem_free_bytes"`
	ContainerMem   uint64    `json:"container_mem_bytes"`
	ContainerLimit uint64    `json:"container_mem_limit_bytes"`
}

// Report is what the status page shows.
type Report struct {
	Now      Sample   `json:"now"`
	Series   []Sample `json:"series"`
	Warnings []string `json:"warnings"`
}

// Options configure the recorder.
type Options struct {
	// DiskPath is the data volume; Disk reads used and total bytes.
	DiskPath string
	Disk     func(path string) (used, total uint64, err error)
	// Root is the filesystem root for /proc and /sys (tests: a temp dir).
	Root string
	// Every is the sampling period (default 5 min); Keep the history (90 d).
	Every, Keep time.Duration
	// DiskWarn is the disk alert share (default 0.75), Warn the memory
	// alert share (default 0.8).
	DiskWarn, Warn float64
	Now            func() time.Time
}

// Recorder samples and keeps history in SQLite.
type Recorder struct {
	o  Options
	db *sql.DB
}

// Open opens (or creates) the history at path.
func Open(path string, o Options) (*Recorder, error) {
	if o.Every <= 0 {
		o.Every = 5 * time.Minute
	}
	if o.Keep <= 0 {
		o.Keep = 90 * 24 * time.Hour
	}
	if o.Warn <= 0 {
		o.Warn = 0.8
	}
	if o.DiskWarn <= 0 {
		o.DiskWarn = 0.75
	}
	if o.Root == "" {
		o.Root = "/"
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS samples(at INTEGER PRIMARY KEY, disk_total INTEGER, disk_used INTEGER, mem_total INTEGER, mem_used INTEGER, mem_free INTEGER, ct_mem INTEGER, ct_limit INTEGER);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Recorder{o: o, db: db}, nil
}

// Close closes the history.
func (r *Recorder) Close() error { return r.db.Close() }

// Read takes one sample now.
func (r *Recorder) Read() Sample {
	s := Sample{At: r.o.Now().UTC()}
	if r.o.Disk != nil {
		if used, total, err := r.o.Disk(r.o.DiskPath); err == nil && total >= used {
			s.DiskTotal, s.DiskUsed, s.DiskFree = total, used, total-used
		}
	}
	mi := meminfo(filepath.Join(r.o.Root, "proc", "meminfo"))
	if t := mi["MemTotal"]; t > 0 {
		avail := mi["MemAvailable"]
		s.MemTotal, s.MemFree = t, avail
		if t >= avail {
			s.MemUsed = t - avail
		}
	}
	cg := filepath.Join(r.o.Root, "sys", "fs", "cgroup")
	s.ContainerMem = readUint(filepath.Join(cg, "memory.current"))
	s.ContainerLimit = readUint(filepath.Join(cg, "memory.max"))
	return s
}

func meminfo(path string) map[string]uint64 {
	out := map[string]uint64{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		if len(fields) > 1 && fields[1] == "kB" {
			n *= 1024
		}
		out[name] = n
	}
	return out
}

func readUint(path string) uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64) // "max" reads as 0
	return n
}

// Record takes a sample, stores it and drops history past Keep.
func (r *Recorder) Record(ctx context.Context) Sample {
	s := r.Read()
	_, _ = r.db.ExecContext(ctx, `INSERT OR REPLACE INTO samples VALUES(?,?,?,?,?,?,?,?)`, s.At.Unix(), s.DiskTotal, s.DiskUsed, s.MemTotal, s.MemUsed, s.MemFree, s.ContainerMem, s.ContainerLimit)
	_, _ = r.db.ExecContext(ctx, `DELETE FROM samples WHERE at<?`, s.At.Add(-r.o.Keep).Unix())
	return s
}

// Report answers the current sample, the history within window (at most
// 500 points, thinned evenly) and the warnings.
func (r *Recorder) Report(ctx context.Context, window time.Duration) Report {
	rep := Report{Now: r.Read(), Series: []Sample{}, Warnings: []string{}}
	rows, err := r.db.QueryContext(ctx, `SELECT at, disk_total, disk_used, mem_total, mem_used, mem_free, ct_mem, ct_limit FROM samples WHERE at>=? ORDER BY at`, r.o.Now().Add(-window).Unix())
	if err == nil {
		for rows.Next() {
			var at int64
			var s Sample
			if rows.Scan(&at, &s.DiskTotal, &s.DiskUsed, &s.MemTotal, &s.MemUsed, &s.MemFree, &s.ContainerMem, &s.ContainerLimit) == nil {
				s.At = time.Unix(at, 0).UTC()
				if s.DiskTotal >= s.DiskUsed {
					s.DiskFree = s.DiskTotal - s.DiskUsed
				}
				rep.Series = append(rep.Series, s)
			}
		}
		rows.Close()
	}
	if n := len(rep.Series); n > 500 {
		thin := make([]Sample, 0, 500)
		for i := range 500 {
			thin = append(thin, rep.Series[i*n/500])
		}
		rep.Series = thin
	}
	if t := rep.Now.DiskTotal; t > 0 && float64(rep.Now.DiskUsed)/float64(t) >= r.o.DiskWarn {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("disk_usage_high: %.0f%% used", 100*float64(rep.Now.DiskUsed)/float64(t)))
	}
	if t := rep.Now.MemTotal; t > 0 && float64(rep.Now.MemUsed)/float64(t) >= r.o.Warn {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("memory_usage_high: %.0f%% used", 100*float64(rep.Now.MemUsed)/float64(t)))
	}
	return rep
}

// Run records a sample now and every Every until ctx ends.
func (r *Recorder) Run(ctx context.Context) {
	r.Record(ctx)
	t := time.NewTicker(r.o.Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.Record(ctx)
		}
	}
}
