package downloads

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The faster rule for dead or crawling torrents, off by default: a torrent
// stuck on metadata for some minutes, or slower than some KB/s over some
// hours, fails and its Slots are searched again, before the stall rule would
// act.

// Setting keys of the dead or crawling torrent rule (group links). 0 is off.
const (
	SettingDeadMetadata = "dead_metadata_minutes"
	SettingCrawlKBps    = "crawl_kbps"
	SettingCrawlHours   = "crawl_hours"
)

// crawlMark is where a torrent stood when its speed window started.
type crawlMark struct {
	since time.Time
	done  int64
}

func (st settings) deadOn() bool  { return st.deadMetadata > 0 || st.crawlOn() }
func (st settings) crawlOn() bool { return st.crawlKBps > 0 && st.crawlHours > 0 }

// deadRule fails every torrent the rule catches. The torrents are read from
// their Downloader by hash, never from a background list.
func (m *Manager) deadRule(ctx context.Context) {
	st := m.settings(ctx)
	if !st.deadOn() {
		m.keepCrawlMarks(nil)
		return
	}
	list, err := deadCandidates(ctx, m.Store.Read())
	if err != nil {
		return
	}
	live := map[int64]bool{}
	for did, dls := range byDownloader(list) {
		if ctx.Err() != nil {
			return
		}
		var dr domain.Downloader
		if database.Get(ctx, m.Store.Read(), "downloaders", did, &dr) != nil {
			continue
		}
		e, err := m.engine(ctx, dr)
		if err != nil {
			continue
		}
		jobs, err := readByHash(ctx, e, dr, dls)
		if err != nil {
			continue
		}
		for _, d := range dls {
			j, ok := jobs[strings.ToLower(d.ExternalID)]
			if !ok {
				continue
			}
			live[d.ID] = true
			if reason := m.deadReason(st, d, j); reason != "" {
				m.failDead(ctx, dr, d, reason, st)
			}
		}
	}
	m.keepCrawlMarks(live)
}

// deadReason says why the rule fails d, or "" when it does not.
func (m *Manager) deadReason(st settings, d domain.Download, j Job) string {
	now := m.now()
	if stuckOnMetadata(j) {
		m.dropCrawlMark(d.ID)
		// From the last start or resume, as the stall timer: paused time never counts.
		since := lastMoved(d)
		if st.deadMetadata > 0 && now.Sub(since) >= st.deadMetadata {
			return "stuck on metadata for " + hours(now.Sub(since))
		}
		return ""
	}
	if !st.crawlOn() || j.State != StateDownloading || j.Progress >= 1 || j.Size <= 0 {
		// Waiting or paused time never counts.
		m.dropCrawlMark(d.ID)
		return ""
	}
	done := int64(j.Progress * float64(j.Size))
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.crawl == nil {
		m.crawl = map[int64]crawlMark{}
	}
	mk, ok := m.crawl[d.ID]
	if !ok || done < mk.done {
		m.crawl[d.ID] = crawlMark{now, done}
		return ""
	}
	took := now.Sub(mk.since)
	if took < st.crawlHours {
		return ""
	}
	if float64(done-mk.done)/took.Seconds() < st.crawlKBps*1024 {
		delete(m.crawl, d.ID)
		return fmt.Sprintf("slower than %s KB/s for %s", trimFloat(st.crawlKBps), hours(took))
	}
	m.crawl[d.ID] = crawlMark{now, done}
	return ""
}

// stuckOnMetadata reports a torrent that does not know its files yet.
func stuckOnMetadata(j Job) bool {
	switch j.Detail {
	case "metaDL", "forcedMetaDL", "metadata":
		return true
	}
	return j.State == StateDownloading && j.Size <= 0 && j.Progress == 0 && len(j.Files) == 0
}

func (m *Manager) dropCrawlMark(id int64) {
	m.mu.Lock()
	delete(m.crawl, id)
	m.mu.Unlock()
}

// keepCrawlMarks drops the marks of downloads not in live.
func (m *Manager) keepCrawlMarks(live map[int64]bool) {
	m.mu.Lock()
	for id := range m.crawl {
		if !live[id] {
			delete(m.crawl, id)
		}
	}
	m.mu.Unlock()
}
