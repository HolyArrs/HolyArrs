package downloads

import (
	"context"
	"log"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// An import that stopped for a reason that may pass on its own (files still
// being copied in, a fault that passed) is tried again until it works or the
// download is gone, as Sonarr and Radarr look at a blocked download again on
// each pass. Each try reads the folder and asks the Downloader, so the waits
// grow for passing faults: retryWaits by try, then the last one. Missing
// or partly copied files are checked every minute until they arrive.
var retryWaits = []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour}

// retryWait is how long the next try waits after the tries already made.
func retryWait(tries int) time.Duration {
	return retryWaits[min(tries, len(retryWaits)-1)]
}

// retryMark is how often a download's waiting import was tried again on its
// own, and when the last try was.
type retryMark struct {
	tries int
	last  time.Time
}

// retryPlan says when each download with a waiting import is tried again on
// its own. Only the Slots that stopped for a reason that may pass are tried;
// the others of the same download keep waiting for a person. The wait runs
// from the later of the newest such Slot's last change and the last try. A
// download with no such Slot is not in the plan. marks, when given, is kept
// to the downloads still waiting, so a download that settled starts afresh.
func retryPlan(waiting []mediafiles.WaitingImport, marks map[int64]retryMark) map[int64]time.Time {
	newest := map[int64]time.Time{}
	files := map[int64]bool{}
	listed := map[int64]bool{}
	for _, w := range waiting {
		listed[w.DownloadID] = true
		if !importer.RetriesOnItsOwn(w.Reason) {
			continue
		}
		files[w.DownloadID] = files[w.DownloadID] || importer.FilesPending(w.Reason)
		if t, ok := newest[w.DownloadID]; !ok || w.Since.After(t) {
			newest[w.DownloadID] = w.Since
		}
	}
	for id := range marks {
		if !listed[id] {
			delete(marks, id)
		}
	}
	plan := map[int64]time.Time{}
	for id, since := range newest {
		mk := marks[id]
		from := since
		if mk.last.After(from) {
			from = mk.last
		}
		wait := retryWait(mk.tries)
		if files[id] && mk.tries < 60 {
			wait = time.Minute
		}
		plan[id] = from.Add(wait)
	}
	return plan
}

// retryDue picks the downloads whose try is due at now, in Slot order.
func retryDue(waiting []mediafiles.WaitingImport, marks map[int64]retryMark, now time.Time) []int64 {
	plan := retryPlan(waiting, marks)
	var due []int64
	seen := map[int64]bool{}
	for _, w := range waiting {
		at, ok := plan[w.DownloadID]
		if !ok || seen[w.DownloadID] || at.After(now) {
			continue
		}
		seen[w.DownloadID] = true
		due = append(due, w.DownloadID)
	}
	return due
}

// RetryTimes says when each download with a waiting import is tried again on
// its own: the same plan the retry pass follows.
func (m *Manager) RetryTimes(waiting []mediafiles.WaitingImport) map[int64]time.Time {
	m.mu.Lock()
	marks := make(map[int64]retryMark, len(m.retried))
	for id, mk := range m.retried {
		marks[id] = mk
	}
	m.mu.Unlock()
	return retryPlan(waiting, marks)
}

// retryWaitingImports tries again the imports that may have passed their fault.
func (m *Manager) retryWaitingImports(ctx context.Context) {
	waiting, err := mediafiles.WaitingImports(ctx, m.Store.Read())
	if err != nil {
		log.Printf("downloads: could not read waiting imports: %v", err)
		return
	}
	now := m.now()
	m.mu.Lock()
	if m.retried == nil {
		m.retried = map[int64]retryMark{}
	}
	due := retryDue(waiting, m.retried, now)
	first := map[int64]bool{}
	for _, id := range due {
		mk := m.retried[id]
		first[id] = mk.tries == 0
		m.retried[id] = retryMark{tries: mk.tries + 1, last: now}
	}
	m.mu.Unlock()
	// One line per download, on its first try: the tries after it are the same.
	for _, id := range due {
		if first[id] {
			log.Printf("downloads: the import of download %d stopped on a fault that may pass; trying it again until it works", id)
		}
		if err := m.importAgainWith(ctx, id, domain.System, importer.RetriesOnItsOwn); err != nil && first[id] && ctx.Err() == nil {
			log.Printf("downloads: trying the import of download %d again: %v", id, err)
		}
	}
}

// retryTimes sets RetryAt on each card whose import is tried again on its
// own, from the plan the retry pass follows for its download. A card that
// waits for a person, or a Manager not running, sets none.
func (a *Web) retryTimes(ctx context.Context, out []DecisionView) {
	if a.Downloads == nil || len(out) == 0 {
		return
	}
	waiting, err := mediafiles.WaitingImports(ctx, a.Store.Read())
	if err != nil {
		return
	}
	plan := a.Downloads.RetryTimes(waiting)
	cardAt := retryCards(waiting, plan)
	for i := range out {
		if at, ok := cardAt[out[i].SlotID]; ok {
			out[i].RetryAt = &at
		}
	}
}

// retryCards is the try time of each waiting Slot that the next try of its
// download takes up: only Slots whose own reason may pass.
func retryCards(waiting []mediafiles.WaitingImport, plan map[int64]time.Time) map[int64]time.Time {
	out := map[int64]time.Time{}
	for _, w := range waiting {
		at, ok := plan[w.DownloadID]
		if ok && importer.RetriesOnItsOwn(w.Reason) {
			out[w.SlotID] = at
		}
	}
	return out
}
