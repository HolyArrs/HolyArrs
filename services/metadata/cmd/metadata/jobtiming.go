package main

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// jobTiming sums picture-queue jobs per minute (count, outcome, time per
// phase) and logs one line a minute, so the queue's speed can be read.
type jobTiming struct {
	mu    sync.Mutex
	start time.Time
	n     int
	byOut map[string]int
	byJob map[string]int
	total time.Duration
	max   time.Duration
	ph    [imgstore.NumPhases]time.Duration
	// extra adds to each line (the hosts' pace waits).
	extra func() string
	now   func() time.Time
	logf  func(string, ...any)
}

func (j *jobTiming) add(job, outcome string, total time.Duration, tr *imgstore.Trace) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.now == nil {
		j.now, j.logf = time.Now, log.Printf
	}
	now := j.now()
	if j.start.IsZero() {
		j.start = now
	}
	if j.byOut == nil {
		j.byOut, j.byJob = map[string]int{}, map[string]int{}
	}
	j.n++
	j.byOut[outcome]++
	j.byJob[job]++
	j.total += total
	j.max = max(j.max, total)
	if tr != nil {
		for i, d := range tr.D {
			j.ph[i] += d
		}
	}
	if now.Sub(j.start) >= time.Minute {
		line := j.lineLocked(now)
		if j.extra != nil {
			if x := j.extra(); x != "" {
				line += " | pace waits " + x
			}
		}
		j.logf("%s", line)
		j.start, j.n, j.byOut, j.byJob = now, 0, nil, nil
		j.total, j.max, j.ph = 0, 0, [imgstore.NumPhases]time.Duration{}
	}
}

func (j *jobTiming) lineLocked(now time.Time) string {
	avg := func(d time.Duration) int64 { return d.Milliseconds() / int64(max(j.n, 1)) }
	var b strings.Builder
	fmt.Fprintf(&b, "images jobs: %d in %s (%.2f/s) %s %s avg ms total=%d max=%d",
		j.n, now.Sub(j.start).Round(time.Second), float64(j.n)/now.Sub(j.start).Seconds(),
		counts(j.byJob), counts(j.byOut), avg(j.total), j.max.Milliseconds())
	for i, d := range j.ph {
		fmt.Fprintf(&b, " %s=%d", imgstore.PhaseNames[i], avg(d))
	}
	return b.String()
}

func counts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return "[" + strings.Join(parts, " ") + "]"
}
