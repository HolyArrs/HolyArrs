package health

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// keep stores the results and writes an Event for every break or recovery
// that held for the flap guard's number of runs. With kinds set (a re-run
// of one group after a fix), only rows of those kinds are dropped when
// gone, and a recovery is confirmed at once: the person just fixed it.
func (e *Engine) keep(ctx context.Context, rs []Result, kinds map[string]bool) ([]domain.HealthCheck, error) {
	guard := e.intSetting(ctx, settingGuard, 2, 1, 20)
	now := e.now()
	var events []*domain.Event
	err := e.Store.Write(ctx, func(tx *sql.Tx) error {
		events = nil
		old, err := database.Select[domain.HealthCheck](ctx, tx, "health_checks", "1=1")
		if err != nil {
			return err
		}
		byKey := map[string]domain.HealthCheck{}
		for _, c := range old {
			byKey[c.Key] = c
		}
		seen := map[string]bool{}
		for _, r := range rs {
			seen[r.Key] = true
			c, had := byKey[r.Key]
			if !had {
				c = domain.HealthCheck{Key: r.Key, Since: now, Confirmed: OK}
			}
			if had && c.State == r.State {
				c.Streak++
			} else {
				c.Streak, c.Since = 1, now
			}
			c.Kind, c.Subject, c.State, c.Message, c.CheckedAt = r.Kind, r.Subject, r.State, r.Message, now
			fixed := kinds != nil && c.State == OK
			if c.State != c.Confirmed && (c.Streak >= guard || fixed) {
				ev := &domain.Event{Kind: domain.EventHealthBroken, Summary: c.Message,
					Data:     map[string]any{"check": c.Key, "kind": c.Kind, "subject": c.Subject, "state": c.State, "message": c.Message, "was": c.Confirmed},
					GroupKey: "health:" + c.Key}
				if c.State == OK {
					ev.Kind, ev.Summary = domain.EventHealthRecovered, "Fixed: "+label(c.Kind)+" "+c.Subject
				}
				events = append(events, ev)
				c.Confirmed = c.State
			}
			if had {
				if _, err := tx.Exec(`UPDATE health_checks SET kind = ?, subject = ?, state = ?, message = ?, since = ?, checked_at = ?, streak = ?, confirmed = ? WHERE id = ?`,
					c.Kind, c.Subject, c.State, c.Message, database.FormatTime(c.Since), database.FormatTime(c.CheckedAt), c.Streak, c.Confirmed, c.ID); err != nil {
					return err
				}
			} else if _, err := tx.Exec(`INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				c.Key, c.Kind, c.Subject, c.State, c.Message, database.FormatTime(c.Since), database.FormatTime(c.CheckedAt), c.Streak, c.Confirmed); err != nil {
				return err
			}
		}
		// Things that are gone (a removed Downloader) drop their check. A
		// trailers check stays while nothing reports trailers yet (at start,
		// before the reader is wired), so a kept warning is not dropped and
		// announced again.
		for k, c := range byKey {
			if seen[k] || (kinds != nil && !kinds[c.Kind]) || (c.Kind == "trailers" && e.Trailers == nil) {
				continue
			}
			if _, err := tx.Exec("DELETE FROM health_checks WHERE id = ?", c.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		if err := e.Store.AddEvent(ctx, ev); err != nil {
			return nil, err
		}
		if e.Publish != nil {
			e.Publish("event", ev)
		}
	}
	cs, err := List(ctx, e.Store)
	if err == nil && e.Publish != nil {
		e.Publish("health", map[string]any{"checks": len(cs)})
	}
	return cs, err
}
