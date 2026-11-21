package main

import (
	"maps"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// workerPools are the worker counts the queue starts with: the core pool
// alone in safe mode, and the YouTube pool too while the youtube feature
// switch is on.
func workerPools(safeMode bool) map[jobs.Pool]int {
	if safeMode {
		return map[jobs.Pool]int{jobs.PoolCore: jobs.DefaultPools[jobs.PoolCore]}
	}
	pools := maps.Clone(jobs.DefaultPools)
	if startup.FeatureOn(startup.YouTube) {
		pools[jobs.PoolYouTube] = 1
	}
	return pools
}

// poolLabels name each pool's stage on the health page; the YouTube pool
// has a stage only while the youtube feature switch is on.
func poolLabels() map[jobs.Pool]string {
	labels := map[jobs.Pool]string{jobs.PoolCore: "Core workers", jobs.PoolIO: "Search and download workers", jobs.PoolHeavy: "Import and scan workers",
		jobs.PoolMigrate: "Import from other apps worker", jobs.PoolTranslate: "Subtitle translation worker"}
	if startup.FeatureOn(startup.YouTube) {
		labels[jobs.PoolYouTube] = "YouTube download worker"
	}
	return labels
}
