package jobs

import (
	"sort"
	"strings"
)

const (
	PoolCore  Pool = "core"
	PoolIO    Pool = "io"
	PoolHeavy Pool = "heavy"
	// PoolMigrate runs Import from other apps, so it never queues behind
	// imports and scans.
	PoolMigrate Pool = "migrate"
)

// DefaultPools are the worker counts the app starts with.
var DefaultPools = map[Pool]int{PoolCore: 1, PoolIO: 2, PoolHeavy: 1, PoolMigrate: 1}

// kindPools assigns every Job kind to a pool. A kind not listed runs in
// the I/O pool; a test in cmd/holyarrs fails for a registered kind that
// is missing here.
var kindPools = map[string]Pool{
	// core
	"health_check":          PoolCore,
	"check_library_folders": PoolCore,
	"clean_history":         PoolCore,
	// outside input and output
	"search_slot":          PoolIO,
	"search_season":        PoolIO,
	"search_album":         PoolIO,
	"search_missing":       PoolIO,
	"feed_check":           PoolIO,
	"grab_held":            PoolIO,
	"grab":                 PoolIO,
	"subtitle_search":      PoolIO,
	"subtitle_missing":     PoolIO,
	"subtitle_better":      PoolIO,
	"sync_outside_lists":   PoolIO,
	"sync_plex_watchlists": PoolIO,
	"check_bridges":        PoolIO,
	"refresh_title":        PoolIO,
	"refresh_titles":       PoolIO,
	"refresh_shows":        PoolIO,
	"refresh_artists":      PoolIO,
	"artist_pictures":      PoolIO,
	// heavy disk work
	"import_download":     PoolHeavy,
	"scan_title":          PoolHeavy,
	"move_title":          PoolHeavy,
	"rename_files":        PoolHeavy,
	"clean_holding":       PoolHeavy,
	"scan_library_folder": PoolHeavy,
	"adopt_library":       PoolHeavy,
	"migrate_move":        PoolMigrate,
	"migrate_undo":        PoolMigrate,
	"rescan_library":      PoolHeavy,
	"subtitle_look":       PoolHeavy,
	"recheck_pick_rules":  PoolHeavy,
}

// Pool is a group of workers. Core work (health checks) never waits behind
// outside input and output (searches, grabs, subtitle searches, details
// lookups) or heavy disk work (scans, imports, renames).
type Pool string

// PoolOf returns the pool a Job kind runs in.
func PoolOf(kind string) Pool {
	if p, ok := kindPools[kind]; ok {
		return p
	}
	return PoolIO
}

// Assigned reports whether a kind has its own row in the pool table.
func Assigned(kind string) bool { _, ok := kindPools[kind]; return ok }

// Kinds returns the kinds a Handler is registered for.
func (q *Queue) Kinds() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]string, 0, len(q.hand))
	for k := range q.hand {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// poolFilter is the SQL that keeps a pool's kinds: the listed kinds for
// core and heavy, everything else for I/O, and no filter for the one
// shared pool.
func poolFilter(p Pool) (string, []any) {
	if p == "" {
		return "", nil
	}
	var args []any
	for k, kp := range kindPools {
		if (p == PoolIO && kp != PoolIO) || (p != PoolIO && kp == p) {
			args = append(args, k)
		}
	}
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(args)), ", ")
	if p == PoolIO {
		return " AND kind NOT IN (" + marks + ")", args
	}
	return " AND kind IN (" + marks + ")", args
}
