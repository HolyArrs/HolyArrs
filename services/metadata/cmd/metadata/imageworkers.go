package main

import (
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
)

// The picture queue's workers. Measured on 2026-10-29 (live-20261029f):
// with 4 workers about 85% of worker time went to music-video jobs, which
// mostly wait on YouTube's one-a-second pace (about 11 s each), so covers
// and photos ran at about 1.2 a second while the CPU sat idle. Each
// source keeps its own pace (hostPace, Limits) however many workers run.
const (
	// 32 once the queue's claim no longer scanned every row (CPU ~20%):
	// jobs mostly wait on a source's answer, not on its pace.
	imageWorkers = 32
	// bigPriority is the lane of deferred big sizes (store-time and
	// backfill); a page someone looks at asks at P1 instead.
	bigPriority = fetchq.P4Deep
	// defaultImageRate is the picture jobs' start rate (job starts, not
	// calls; every source keeps its own pace, Apple 10 a second).
	defaultImageRate = 20
	// videoJobsMax caps the music-video jobs (hv:, hvsb:) running at once.
	videoJobsMax = 2
)

func isVideoJob(key string) bool {
	return strings.HasPrefix(key, "hv:") || strings.HasPrefix(key, "hvsb:")
}

// holdImage is the picture queue's Hold: a music-video job over
// videoJobsMax goes back for a few seconds; an admitted one counts until
// videoDone.
func (sp *speed) holdImage(key string, _ int) (bool, time.Duration) {
	if !isVideoJob(key) {
		return false, 0
	}
	if sp.videoJobs.Add(1) > videoJobsMax {
		sp.videoJobs.Add(-1)
		return true, 3 * time.Second
	}
	return false, 0
}

func (sp *speed) videoDone() { sp.videoJobs.Add(-1) }
