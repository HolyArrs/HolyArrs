package downloader

import (
	"context"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// SyncView is one client as the background reader (package dlsync) last
// read it.
//
// Complete: the last read returned the whole list without error. Fresh:
// Complete and recent enough to act on. Degraded: the reader is not
// running for this client, or it has not been Fresh for its bound; the
// Manager then reads the client itself. Scope is the category the read
// covered ("" when it read everything).
type SyncView struct {
	Items    map[string]Job
	Scope    string
	Complete bool
	Fresh    bool
	Degraded bool
	ReadAt   time.Time
}

// SyncSource is the background reader as the Manager sees it. With setting
// downloads.sync = on the Manager reads clients through it instead of
// listing them; every other value keeps the Manager's own reads.
type SyncSource interface {
	// On says the reader owns list reads now.
	On() bool
	// View is one client's current snapshot (a copy).
	View(downloaderID int64) SyncView
	// Fallback marks a client the Manager reads itself this tick, so the
	// reader skips its scheduled read and only tries to recover.
	Fallback(downloaderID int64, on bool)
	// Kick asks for a read soon (after a grab or an item action).
	Kick(downloaderID int64)
	// Changed is signalled when any client's snapshot moved.
	Changed() <-chan struct{}
}

// Kick asks the background reader to read a Downloader soon. A no-op
// without one.
func (m *Manager) Kick(downloaderID int64) {
	if m.Sync != nil {
		m.Sync.Kick(downloaderID)
	}
}

// Reader gives the Downloader's guarded engine for background reads, with
// the generation it belongs to.
func (m *Manager) Reader(ctx context.Context, d domain.Downloader) (Engine, uint64, error) {
	gen := m.Generation(d.ID)
	e, err := m.engine(ctx, d)
	return e, gen, err
}

// ReportRead records the outcome of a background read on the Downloader's
// health, the same way the Manager's own reads do.
func (m *Manager) ReportRead(ctx context.Context, d domain.Downloader, err error) {
	if err != nil {
		m.downloaderFailed(ctx, d, err)
		return
	}
	m.downloaderOK(ctx, d)
}

// storeOn says this tick reads clients through the background reader.
func (m *Manager) storeOn() bool {
	return m.Sync != nil && m.Sync.On()
}

// snapshotJobs is a SyncView's items in the Downloader's category, the same
// set the Manager's own List(category) gives.
func snapshotJobs(v SyncView, category string) []Job {
	out := make([]Job, 0, len(v.Items))
	for _, j := range v.Items {
		if v.Scope == "" && category != "" && j.Category != category {
			continue
		}
		out = append(out, j)
	}
	return out
}
