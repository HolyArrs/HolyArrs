package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// OnEvent adds fn to the functions told about every Event once it is
// stored. fn runs on the writer's caller and must return quickly (start a
// goroutine for slow work such as sending an Alert).
func (s *Store) OnEvent(fn func(domain.Event)) {
	s.hookMu.Lock()
	s.hooks = append(s.hooks, fn)
	s.hookMu.Unlock()
}

// AddEvent writes one Event to the events database and sets its ID.
func (s *Store) AddEvent(ctx context.Context, e *domain.Event) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	if e.Actor.Kind == "" {
		e.Actor = domain.System
	}
	err := s.WriteEvents(ctx, func(tx *sql.Tx) error {
		_, err := Insert(tx, "events", e, e.Actor)
		return err
	})
	if err != nil {
		return err
	}
	s.hookMu.RLock()
	hooks := s.hooks
	s.hookMu.RUnlock()
	// A hook that crashes is caught here, so it never takes down the
	// caller that stored the Event.
	for _, fn := range hooks {
		supervisor.Guard("events", func() { fn(*e) })
	}
	return nil
}

// RecentEvents returns the newest Events first.
func (s *Store) RecentEvents(ctx context.Context, limit int) ([]domain.Event, error) {
	es, _, err := s.Events(ctx, EventFilter{Limit: limit})
	return es, err
}

// EventFilter narrows a History list. Zero fields do not filter.
type EventFilter struct {
	TitleID int64
	// Kinds keeps Events of these kinds; a kind ending in "*" is a prefix
	// (import_* is every import outcome).
	Kinds    []string
	From, To time.Time // From is inclusive, To exclusive
	Limit    int       // 1 to 500, default 50
	Offset   int
	// An episode's Events: those of its Slots (the Event's own Slot or a
	// pack's slot_ids) and those naming the episode (item_id, item_ids).
	SlotIDs []int64
	ItemIDs []int64
	// An album's Events also take those naming the album (part_id,
	// album_id).
	PartID int64
}

// Events lists the Events that match f, newest first, and how many match
// in all (for paging).
func (s *Store) Events(ctx context.Context, f EventFilter) ([]domain.Event, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	var where []string
	var args []any
	if f.TitleID > 0 {
		where, args = append(where, "title_id = ?"), append(args, f.TitleID)
	}
	if len(f.Kinds) > 0 {
		var or []string
		for _, k := range f.Kinds {
			if p, ok := strings.CutSuffix(k, "*"); ok {
				or, args = append(or, "kind LIKE ? ESCAPE '\\'"), append(args, strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(p)+"%")
			} else {
				or, args = append(or, "kind = ?"), append(args, k)
			}
		}
		where = append(where, "("+strings.Join(or, " OR ")+")")
	}
	if len(f.SlotIDs) > 0 || len(f.ItemIDs) > 0 {
		var or []string
		if len(f.SlotIDs) > 0 {
			in := strings.TrimSuffix(strings.Repeat("?,", len(f.SlotIDs)), ",")
			or = append(or, "slot_id IN ("+in+")", "EXISTS (SELECT 1 FROM json_each(events.data, '$.slot_ids') j WHERE j.value IN ("+in+"))")
			for range 2 {
				for _, id := range f.SlotIDs {
					args = append(args, id)
				}
			}
		}
		if len(f.ItemIDs) > 0 {
			in := strings.TrimSuffix(strings.Repeat("?,", len(f.ItemIDs)), ",")
			or = append(or, "json_extract(data, '$.item_id') IN ("+in+")", "EXISTS (SELECT 1 FROM json_each(events.data, '$.item_ids') j WHERE j.value IN ("+in+"))")
			for range 2 {
				for _, id := range f.ItemIDs {
					args = append(args, id)
				}
			}
		}
		if f.PartID > 0 {
			or = append(or, "json_extract(data, '$.part_id') = ?", "json_extract(data, '$.album_id') = ?")
			args = append(args, f.PartID, f.PartID)
		}
		where = append(where, "("+strings.Join(or, " OR ")+")")
	} else if f.PartID > 0 {
		where = append(where, "(json_extract(data, '$.part_id') = ? OR json_extract(data, '$.album_id') = ?)")
		args = append(args, f.PartID, f.PartID)
	}
	if !f.From.IsZero() {
		where, args = append(where, "at >= ?"), append(args, FormatTime(f.From))
	}
	if !f.To.IsZero() {
		where, args = append(where, "at < ?"), append(args, FormatTime(f.To))
	}
	cond := "1=1"
	if len(where) > 0 {
		cond = strings.Join(where, " AND ")
	}
	var total int
	if err := s.readEv.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE "+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	var e domain.Event
	rows, err := s.readEv.QueryContext(ctx, "SELECT "+SelectList(&e)+" FROM events WHERE "+cond+" ORDER BY at DESC, id DESC LIMIT ? OFFSET ?",
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.Event{}
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(ScanInto(&e)...); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}
