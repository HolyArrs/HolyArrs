package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// nearCapShare is how full a limit gets before Settings says it is close.
const nearCapShare = 0.8

// NearCap says whether a Source used 80% or more of its searches or picks in its window, with the words to show.
// A limit of 0 means no limit.
func NearCap(ctx context.Context, st *database.Store, src domain.Source, now time.Time) (bool, string) {
	l := src.Limits
	if l.SearchesPerWindow <= 0 && l.GrabsPerWindow <= 0 {
		return false, ""
	}
	window := time.Duration(l.WindowSeconds) * time.Second
	if window <= 0 {
		window = 24 * time.Hour
	}
	since := database.FormatTime(now.Add(-window))
	db := st.Read()
	if l.SearchesPerWindow > 0 {
		if n, err := searchCallsSince(ctx, db, src.ID, since); err == nil &&
			float64(n) >= nearCapShare*float64(l.SearchesPerWindow) {
			return true, fmt.Sprintf("The Source %s is close to its search limit (%d of %d)", src.Name, n, l.SearchesPerWindow)
		}
	}
	if l.GrabsPerWindow > 0 {
		if n, err := picksSince(ctx, db, src.ID, since); err == nil &&
			float64(n) >= nearCapShare*float64(l.GrabsPerWindow) {
			return true, fmt.Sprintf("The Source %s is close to its pick limit (%d of %d)", src.Name, n, l.GrabsPerWindow)
		}
	}
	return false, ""
}
