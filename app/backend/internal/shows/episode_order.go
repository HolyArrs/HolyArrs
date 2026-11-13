package shows

import (
	"context"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// groupSeasons summarises a show's seasons by its chosen episode order.
// Episodes the order leaves out are not counted in any season; specials
// keep their own season. A season keeps the name and Keep up to date
// switch of the stored season with the same number, when there is one.
func groupSeasons(ctx context.Context, db database.Lister, titleID int64, now time.Time, parts []mediafiles.Part) ([]SeasonSummary, error) {
	rows, err := library.OrderEpisodeRows(ctx, db, titleID)
	if err != nil {
		return nil, err
	}
	byNumber := map[int]mediafiles.Part{}
	specials := int64(-1)
	for _, p := range parts {
		n := library.Deref(p.Number)
		byNumber[n] = p
		if n == 0 {
			specials = p.ID
		}
	}
	out := []SeasonSummary{}
	index := map[int]int{}
	add := func(n int, keep bool) int {
		if idx, ok := index[n]; ok {
			return idx
		}
		ss := SeasonSummary{Number: n, Name: "Season " + strconv.Itoa(n), KeepUpToDate: keep}
		if p, ok := byNumber[n]; ok {
			ss.Name, ss.KeepUpToDate, ss.Dropped = p.Name, p.KeepUpToDate, p.DroppedAt != nil
		}
		index[n] = len(out)
		out = append(out, ss)
		return index[n]
	}
	if specials >= 0 {
		add(0, false)
	}
	for _, r := range rows {
		n := r.AltSeason
		switch {
		case n > 0:
		case r.PartID == specials:
			n = 0
		default:
			continue
		}
		out[add(n, r.Keep)].tally(r.AirDate, r.AirDay, r.Keep, r.State, now)
	}
	return out, nil
}
