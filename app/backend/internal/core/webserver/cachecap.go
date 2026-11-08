package webserver

import (
	"slices"
	"time"
)

// KeepUnder makes room for one more entry in m under limit by dropping the
// oldest tenth (by at), so hitting a cap never empties a cache at once.
func KeepUnder[K comparable, V any](m map[K]V, limit int, at func(V) time.Time) {
	if len(m) < limit {
		return
	}
	times := make([]time.Time, 0, len(m))
	for _, v := range m {
		times = append(times, at(v))
	}
	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	drop := max(len(m)-limit+1, limit/10)
	cut := times[min(drop, len(times))-1]
	for k, v := range m {
		if !at(v).After(cut) && drop > 0 {
			delete(m, k)
			drop--
		}
	}
}
