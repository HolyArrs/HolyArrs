package source

import (
	"context"
	"fmt"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// nearCapShare is how full a limit is before Settings says it is close.
const nearCapShare = 0.8

// NearCap says whether a Source used 80% or more of its searches or picks
// in its window, with the words to show. A limit of 0 means no limit.
func NearCap(ctx context.Context, st *store.Store, src domain.Source, now time.Time) (bool, string) {
	l := src.Limits
	if l.SearchesPerWindow <= 0 && l.GrabsPerWindow <= 0 {
		return false, ""
	}
	window := time.Duration(l.WindowSeconds) * time.Second
	if window <= 0 {
		window = 24 * time.Hour
	}
	since := store.FormatTime(now.Add(-window))
	db := st.Read()
	if l.SearchesPerWindow > 0 {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM source_calls WHERE source_id = ? AND op = 'search' AND at > ?", src.ID, since).Scan(&n); err == nil &&
			float64(n) >= nearCapShare*float64(l.SearchesPerWindow) {
			return true, fmt.Sprintf("The Source %s is close to its search limit (%d of %d)", src.Name, n, l.SearchesPerWindow)
		}
	}
	if l.GrabsPerWindow > 0 {
		// The same count overGrabLimit uses: Downloads from its Releases plus
		// Releases chosen and not yet sent.
		var n int
		if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM downloads d JOIN releases r ON r.id = d.release_id WHERE r.source_id = ? AND d.created_at > ?) +
		(SELECT count(*) FROM releases r WHERE r.source_id = ? AND r.state = 'chosen' AND r.updated_at > ?
			AND NOT EXISTS (SELECT 1 FROM downloads d WHERE d.release_id = r.id))`, src.ID, since, src.ID, since).Scan(&n); err == nil &&
			float64(n) >= nearCapShare*float64(l.GrabsPerWindow) {
			return true, fmt.Sprintf("The Source %s is close to its pick limit (%d of %d)", src.Name, n, l.GrabsPerWindow)
		}
	}
	return false, ""
}
