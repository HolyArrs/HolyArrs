package shows

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Show refresh rules (Sonarr's show refresh, backend/jobs.md task 9).
const (
	showStale      = 30 * 24 * time.Hour // always refresh after this
	showFresh      = 6 * time.Hour       // never refresh before this
	showEndedGrace = 30 * 24 * time.Hour // an ended show is refreshed while its last episode is this recent
)

// ShowFacts are what the refresh rules read about a show's episodes.
type ShowFacts struct {
	AiredWithoutName bool       // a regular episode that has aired still has no real name
	LastAired        *time.Time // the latest air time that has passed
}

// placeholderNames are names sources give an episode before the real one.
var placeholderNames = map[string]bool{"": true, "TBA": true, "TBD": true, "To Be Announced": true}

// ShouldRefreshShow applies the show refresh rules, in order: never
// fetched, yes; over 30 days since the last fetch, yes; an aired regular
// episode still without a real name, yes; under 6 hours, no; not ended,
// yes; ended with the last episode in the last 30 days, yes; else no.
func ShouldRefreshShow(t mediafiles.Title, f ShowFacts, now time.Time) (yes bool, why string) {
	if t.RefreshedAt == nil {
		return true, "never fetched"
	}
	age := now.Sub(*t.RefreshedAt)
	switch {
	case age > showStale:
		return true, "last fetch over 30 days ago"
	case f.AiredWithoutName:
		return true, "an aired episode has no name yet"
	case age < showFresh:
		return false, "last fetch under 6 hours ago"
	case t.Status != "ended":
		return true, "still running"
	case f.LastAired != nil && now.Sub(*f.LastAired) <= showEndedGrace:
		return true, "ended, last episode in the last 30 days"
	}
	return false, "ended"
}

// showFacts reads the facts the refresh rules need for one show.
func showFacts(ctx context.Context, db database.Lister, titleID int64, now time.Time) (ShowFacts, error) {
	var f ShowFacts
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(name, ''), air_date FROM items WHERE title_id = ? AND kind = 'episode'
		AND json_extract(numbers, '$.season') > 0 AND air_date IS NOT NULL AND air_date <= ?`, titleID, database.FormatTime(now))
	if err != nil {
		return f, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, air string
		if err := rows.Scan(&name, &air); err != nil {
			return f, err
		}
		if placeholderNames[name] {
			f.AiredWithoutName = true
		}
		if t, err := time.Parse(database.TimeLayout, air); err == nil && (f.LastAired == nil || t.After(*f.LastAired)) {
			f.LastAired = &t
		}
	}
	return f, rows.Err()
}

// KindRefreshShows is the 12-hour show refresh, or every show by hand.
const KindRefreshShows = "refresh_shows"

// ShowRefreshPlan picks the shows a refresh-all run covers: up to
// library.ScoreFillPerRun shows with no score yet, then what the rules say.
func (s *Service) ShowRefreshPlan(ctx context.Context, byHand bool) (ids []int64, why map[int64]string, err error) {
	db := s.Store.Read()
	ts, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind = 'show' AND state != 'removed' ORDER BY sort_name, id")
	if err != nil {
		return nil, nil, err
	}
	why = map[int64]string{}
	now := s.Clock()
	filled := 0
	for _, t := range ts {
		if byHand {
			ids, why[t.ID] = append(ids, t.ID), "by hand"
			continue
		}
		if filled < library.ScoreFillPerRun && library.NeedsScore(t, now, showFresh) {
			ids, why[t.ID], filled = append(ids, t.ID), "no score yet", filled+1
			continue
		}
		f, err := showFacts(ctx, db, t.ID, now)
		if err != nil {
			return nil, nil, err
		}
		if yes, w := ShouldRefreshShow(t, f, now); yes {
			ids, why[t.ID] = append(ids, t.ID), w
		}
	}
	return ids, why, nil
}

// OpenEpisodeGates moves episode Slots whose air time (plus the wait) has
// passed from not_out_yet to wanted. It reads first and writes only when a
// Slot crosses, so an idle Library causes no disk writes.
func (s *Service) OpenEpisodeGates(ctx context.Context) (int, error) {
	db := s.Store.Read()
	defs, err := library.ReadShowDefaults(ctx, db)
	if err != nil {
		return 0, err
	}
	now := s.Clock()
	wait := time.Duration(defs.WaitMinutes) * time.Minute
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT i.title_id FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE i.kind = 'episode' AND s.state = 'not_out_yet' AND t.state != 'removed' AND i.air_date IS NOT NULL AND i.air_date <= ?`,
		database.FormatTime(now.Add(-wait)))
	if err != nil {
		return 0, err
	}
	var titles []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		titles = append(titles, id)
	}
	rows.Close()
	n := 0
	for _, id := range titles {
		var opened []int64
		var t mediafiles.Title
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			if err := database.Get(ctx, tx, "titles", id, &t); err != nil {
				return err
			}
			var err error
			opened, _, err = episodeGatesTx(ctx, tx, id, wait, now, domain.System)
			return err
		})
		if err != nil {
			return n, err
		}
		n += len(opened)
		s.episodeGateEvents(ctx, t, opened, nil, domain.System, nil)
	}
	return n, nil
}

// handleRefreshShows refreshes the planned shows one by one. A failure on
// one show is recorded and the run goes on.
func (s *Service) handleRefreshShows(ctx context.Context, r *jobs.Run) error {
	byHand, _ := r.Job.Params["by_hand"].(bool)
	ids, why, err := s.ShowRefreshPlan(ctx, byHand)
	if err != nil {
		return err
	}
	var failed []string
	dropped, added := 0, 0
	for i, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = r.Progress(ctx, float64(i)/float64(len(ids)), fmt.Sprintf("refreshing show %d of %d", i+1, len(ids)))
		out, err := s.RefreshShow(ctx, id, library.JobActor(r), &r.Job.ID)
		if err != nil {
			if errors.Is(err, metadata.ErrNoAccess) && s.allOn(ctx, "tmdb") {
				return err // every other show would fail the same way
			}
			failed = append(failed, fmt.Sprintf("title %d: %v", id, err))
			continue
		}
		if out.Dropped {
			dropped++
		}
		added += out.EpisodesAdded
	}
	reasons := map[string]int{}
	for _, w := range why {
		reasons[w]++
	}
	r.SetResult(map[string]any{"refreshed": len(ids) - len(failed), "failed": failed, "dropped": dropped, "episodes_added": added, "why": reasons})
	return nil
}

// allOn reports whether every show follows source.
func (s *Service) allOn(ctx context.Context, source string) bool {
	var n int
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM titles WHERE kind = 'show' AND state != 'removed' AND metadata_source != ?", source).Scan(&n)
	return n == 0
}
