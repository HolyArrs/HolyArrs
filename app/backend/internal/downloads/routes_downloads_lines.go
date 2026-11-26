package downloads

import (
	"database/sql"
	"strings"
	"time"
)

// releaseLine is a Release's name and size.
type releaseLine struct {
	name     string
	size     int64
	source   string
	sourceID int64
	posted   *time.Time
}

// inChunks calls f on ids in parts small enough for one IN (...) read.
func inChunks(ids []int64, f func(part []any, marks string)) {
	seen := map[int64]bool{}
	uniq := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	for len(uniq) > 0 {
		n := min(len(uniq), 500)
		part := make([]any, n)
		for i, id := range uniq[:n] {
			part[i] = id
		}
		uniq = uniq[n:]
		f(part, "?"+strings.Repeat(", ?", n-1))
	}
}

// slotInfo is what the Downloads screen shows about one Slot: its episode
// line, and its Title, Version and (for tracks) album.
type slotInfo struct {
	line      EpisodeLine
	titleID   int64
	title     string
	year      sql.NullInt64
	kind      string
	version   string
	full      bool // the Slot has its Version (the Title line is shown)
	album     sql.NullString
	albumYear sql.NullString
	active    sql.NullInt64 // the download the Slot waits on now
}

// linesOf is the episode lines of a download's Slots, in order. A Slot that
// now waits on another download (an episode sent again after this one left
// it out) is marked, so this download does not wait on it.
func linesOf(downloadID int64, ids []int64, slots map[int64]slotInfo) []EpisodeLine {
	out := []EpisodeLine{}
	for _, id := range ids {
		if si, ok := slots[id]; ok {
			si.line.elsewhere = si.active.Valid && si.active.Int64 != downloadID
			out = append(out, si.line)
		}
	}
	SortLines(out)
	return out
}
