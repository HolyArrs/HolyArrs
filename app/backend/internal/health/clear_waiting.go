package health

import (
	"context"
	"sort"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// clearing warns while imported downloads still wait to be cleared from
// their Downloader after a failed try, with the last reason given; it is ok
// again once every clear worked or ended.
func (e *Engine) clearing(ctx context.Context) ([]Result, error) {
	r := Result{Key: "clear_waiting", Kind: "clear_waiting", Subject: "Downloads", State: OK}
	waits := downloads.ClearWaits(ctx, e.Store.Read())
	if len(waits) == 0 {
		return []Result{r}, nil
	}
	ids := make([]int64, 0, len(waits))
	for id := range waits {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	// The reason of the clear that failed most often is the one to read.
	why, most := "", -1
	for _, id := range ids {
		if w := waits[id]; w.Reason != "" && w.Tries > most {
			why, most = w.Reason, w.Tries
		}
	}
	r.State = Warning
	if len(waits) == 1 {
		r.Message = "1 imported download is still waiting to be cleared from its Downloader. HolyArrs tries again on its own"
	} else {
		r.Message = strconv.Itoa(len(waits)) + " imported downloads are still waiting to be cleared from their Downloader. HolyArrs tries again on its own"
	}
	if why != "" {
		r.Message += "; the last try said: " + why
	}
	return []Result{r}, nil
}
