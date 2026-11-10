package downloads

import (
	"context"
	"slices"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// SyncView is a read of one Downloader for the sync loop.
type SyncView struct {
	Items    map[string]Job
	Scope    string
	Complete bool
	Fresh    bool
	Degraded bool
	ReadAt   time.Time
}

// SyncSource gives the sync loop access to Downloaders.
type SyncSource interface {
	On() bool
	View(downloaderID int64) SyncView
	Fallback(downloaderID int64, on bool)
	Kick(downloaderID int64)
	Changed() <-chan struct{}
}

// Kick asks the sync loop to refresh the Downloader now.
func (m *Manager) Kick(downloaderID int64) {
	if m.Sync != nil {
		m.Sync.Kick(downloaderID)
	}
}

// Reader returns the engine and generation to read d with.
func (m *Manager) Reader(ctx context.Context, d domain.Downloader) (Engine, uint64, error) {
	gen := m.Generation(d.ID)
	e, err := m.engine(ctx, d)
	return e, gen, err
}

// ReportRead records the outcome of a read of d.
func (m *Manager) ReportRead(ctx context.Context, d domain.Downloader, err error) {
	if err != nil {
		m.downloaderFailed(ctx, d, err)
		return
	}
	m.downloaderOK(ctx, d)
}

func (m *Manager) storeOn() bool {
	return m.Sync != nil && m.Sync.On()
}

func snapshotJobs(v SyncView, cats []string) []Job {
	out := make([]Job, 0, len(v.Items))
	for _, j := range v.Items {
		if v.Scope == "" && !slices.Contains(cats, "") && !slices.Contains(cats, j.Category) {
			continue
		}
		out = append(out, j)
	}
	return out
}
