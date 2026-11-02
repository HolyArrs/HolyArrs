package refresh

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// Fetcher reads one upstream query (the server's budgeted client).
type Fetcher interface {
	Fetch(context.Context, model.Query) (model.Result, error)
}

const (
	tmdbCursor = "cursor:tmdb"
	maxWindow  = 14 * 24 * time.Hour
	dateLayout = "2006-01-02"
)

// TMDBPoller reads the TMDB change lists and marks the held rows stale.
type TMDBPoller struct {
	Tracker *Tracker
	Fetcher Fetcher
	Now     func() time.Time
	// MaxPages bounds pages per list per run (default 100).
	MaxPages int
}

// Poll runs once: from the saved cursor (overlapping a day, since TMDB
// dates are day-granular) to today, in windows of at most 14 days. The
// cursor moves only after every list succeeded. A cursor older than 14
// days (a long outage) marks every TMDB row verified before it stale.
func (p *TMDBPoller) Poll(ctx context.Context) (marked int, err error) {
	now := p.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	saved := p.Tracker.Get(ctx, tmdbCursor)
	if saved == "" {
		// First run: start from yesterday.
		return 0, p.Tracker.Set(ctx, tmdbCursor, today.Add(-24*time.Hour).Format(dateLayout))
	}
	cursor, err := time.Parse(dateLayout, saved)
	if err != nil {
		return 0, p.Tracker.Set(ctx, tmdbCursor, today.Format(dateLayout))
	}
	if today.Sub(cursor) > maxWindow {
		n, err := p.Tracker.MarkPrefixStale(ctx, "tmdb:", cursor.Add(24*time.Hour))
		if err != nil {
			return 0, err
		}
		p.Tracker.Set(ctx, "feed:tmdb", "recovered "+now.Format(time.RFC3339))
		return n, p.Tracker.Set(ctx, tmdbCursor, today.Format(dateLayout))
	}
	start := cursor.Add(-24 * time.Hour)
	for _, kind := range []string{"movie", "tv", "person"} {
		ids, err := p.changes(ctx, kind, start, today)
		if err != nil {
			p.Tracker.Set(ctx, "feed:tmdb", "failed "+now.Format(time.RFC3339))
			return marked, err
		}
		refs := make([]string, 0, len(ids))
		for _, id := range ids {
			refs = append(refs, "tmdb:"+kind+":"+strconv.FormatInt(id, 10))
		}
		n, err := p.Tracker.MarkRefs(ctx, refs)
		if err != nil {
			return marked, err
		}
		marked += n
	}
	p.Tracker.Set(ctx, "feed:tmdb", "ok "+now.Format(time.RFC3339))
	return marked, p.Tracker.Set(ctx, tmdbCursor, today.Format(dateLayout))
}

func (p *TMDBPoller) changes(ctx context.Context, kind string, start, end time.Time) ([]int64, error) {
	maxPages := p.MaxPages
	if maxPages <= 0 {
		maxPages = 100
	}
	var ids []int64
	for page := 1; page <= maxPages; page++ {
		v := url.Values{"start_date": {start.Format(dateLayout)}, "end_date": {end.Format(dateLayout)}, "page": {strconv.Itoa(page)}}
		q := model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: "/" + kind + "/changes", Page: 1, Limit: 20}
		var err error
		if q.Params, err = model.TMDBRead(q.Path, v); err != nil {
			return nil, err
		}
		res, err := p.Fetcher.Fetch(ctx, q)
		if err != nil {
			return nil, err
		}
		var body struct {
			Results []struct {
				ID int64 `json:"id"`
			} `json:"results"`
			TotalPages int `json:"total_pages"`
		}
		if err := json.Unmarshal(res.Data, &body); err != nil {
			return nil, errors.New("tmdb changes: bad answer")
		}
		for _, r := range body.Results {
			if r.ID > 0 {
				ids = append(ids, r.ID)
			}
		}
		if page >= body.TotalPages {
			break
		}
	}
	return ids, nil
}
