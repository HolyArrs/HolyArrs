package sources

import (
	"context"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ReuseWindow is how long a Slot's last search results may answer the
// search that follows a failed Download. Within it the next Release is
// picked from those results, so no Source is asked again; after it, or when
// none of them passes, every Source is searched as usual. The old apps
// always search again.
const ReuseWindow = 15 * time.Minute

// reuseParam marks a search queued after a failed Download.
const reuseParam = "reuse_recent"

// recentFound turns the Releases of the Slot's newest search back into
// candidates when that search is younger than ReuseWindow. Releases turned
// down, and those from a Source that is no longer enabled, are left out.
func (s *Searcher) recentFound(ctx context.Context, slotID int64, now time.Time) []found {
	db := s.Store.Read()
	var jobID int64
	var newest string
	err := db.QueryRowContext(ctx, `SELECT r.job_id, max(r.created_at) FROM releases r JOIN json_each(r.matched_slot_ids) j
		WHERE j.value = ? AND r.job_id IS NOT NULL AND r.found_by != 'feed'
		GROUP BY r.job_id ORDER BY r.job_id DESC LIMIT 1`, slotID).Scan(&jobID, &newest)
	if err != nil || newest < database.FormatTime(now.Add(-ReuseWindow)) {
		return nil
	}
	rels, err := database.Select[domain.Release](ctx, db, "releases", "job_id = ? AND state != 'turned_down' ORDER BY id", jobID)
	if err != nil {
		return nil
	}
	srcs := map[int64]*domain.Source{}
	var out []found
	for _, rel := range rels {
		if turnedDownFor(rel, slotID) {
			continue
		}
		src, ok := srcs[rel.SourceID]
		if !ok {
			var got domain.Source
			if database.Get(ctx, db, "sources", rel.SourceID, &got) == nil && got.State == "enabled" {
				src = &got
			}
			srcs[rel.SourceID] = src
		}
		if src == nil {
			continue
		}
		it := Item{Title: rel.Name, Link: rel.LinkRef, Size: rel.Size, Protocol: rel.Protocol, Seeders: rel.Seeders, Peers: rel.Peers}
		if rel.PublishedAt != nil {
			it.Published = *rel.PublishedAt
		}
		if rel.InfoHash != nil {
			it.InfoHash = *rel.InfoHash
		}
		it.Languages = storedSourceLanguages(rel.Parsed)
		out = append(out, found{src: *src, item: it, relID: rel.ID, byID: matchedByID(rel)})
	}
	return out
}
