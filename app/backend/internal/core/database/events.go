package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"sync/atomic"
	"time"
)

// eventGuard runs a hook so that a crash in it never reaches the caller. By
// default it logs the crash; the program sets it to the supervisor's guard at
// start so the crash also shows in the health stages. It is read by every
// AddEvent and written by each start, so it sits behind an atomic pointer.
var eventGuard atomic.Pointer[func(name string, f func())]

// guardHook is the guard used until the program sets one. Only the panic's
// type is logged, on purpose: its text may hold a secret.
func guardHook(name string, f func()) {
	if g := eventGuard.Load(); g != nil {
		(*g)(name, f)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("database: %s hook crashed: %T", name, r)
		}
	}()
	f()
}

// Event is one line of history. Events live in the events database.
type Event struct {
	Meta
	At         time.Time      `db:"at"`
	Actor      Actor          `db:"actor,json"`
	Kind       string         `db:"kind"`
	TitleID    *int64         `db:"title_id"`
	SlotID     *int64         `db:"slot_id"`
	DownloadID *int64         `db:"download_id"`
	JobID      *int64         `db:"job_id"`
	Summary    string         `db:"summary"`
	Data       map[string]any `db:"data,json"`
	GroupKey   string         `db:"group_key"`
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
	// SourceID keeps the Events of one Source: grabs whose Release came
	// from it (data.release_id, one of SourceReleaseIDs) and those naming it
	// (data.source_id). The caller reads the Release ids from the Source's
	// owner, because Releases live in the main database and Events in their
	// own.
	SourceID         int64
	SourceReleaseIDs []int64
	// HideFeed drops the routine feed checks.
	HideFeed bool
	// HideKinds drops Events of these exact kinds (from the list and the
	// count), whatever Kinds asks for.
	HideKinds []string
}

// SetEventGuard sets the function that runs each event hook.
func SetEventGuard(g func(name string, f func())) { eventGuard.Store(&g) }

// OnEvent adds fn to the functions told about every Event once it is
// stored. fn runs on the writer's caller and must return quickly (start a
// goroutine for slow work such as sending an Alert).
func (s *Store) OnEvent(fn func(Event)) {
	s.hookMu.Lock()
	s.hooks = append(s.hooks, fn)
	s.hookMu.Unlock()
}

// AddEvent writes one Event to the events database and sets its ID.
func (s *Store) AddEvent(ctx context.Context, e *Event) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	if e.Actor.Kind == "" {
		e.Actor = System
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
		guardHook("events", func() { fn(*e) })
	}
	return nil
}

// RecentEvents returns the newest Events first.
func (s *Store) RecentEvents(ctx context.Context, limit int) ([]Event, error) {
	es, _, err := s.Events(ctx, EventFilter{Limit: limit})
	return es, err
}

// Events lists the Events that match f, newest first, and how many match
// in all (for paging).
func (s *Store) Events(ctx context.Context, f EventFilter) ([]Event, int, error) {
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
	if f.SourceID > 0 {
		ids, err := json.Marshal(append([]int64{}, f.SourceReleaseIDs...))
		if err != nil {
			return nil, 0, err
		}
		where = append(where, "(json_extract(data, '$.release_id') IN (SELECT value FROM json_each(?)) OR json_extract(data, '$.source_id') = ?)")
		args = append(args, string(ids), f.SourceID)
	}
	if len(f.HideKinds) > 0 {
		where = append(where, "kind NOT IN ("+strings.TrimSuffix(strings.Repeat("?,", len(f.HideKinds)), ",")+")")
		for _, k := range f.HideKinds {
			args = append(args, k)
		}
	}
	if f.HideFeed {
		where = append(where, "kind <> 'feed_checked'")
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
	var e Event
	rows, err := s.readEv.QueryContext(ctx, "SELECT "+SelectList(&e)+" FROM events WHERE "+cond+" ORDER BY at DESC, id DESC LIMIT ? OFFSET ?",
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(ScanInto(&e)...); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// BumpFailedSignIn adds one to the newest failed sign-in row from the same
// address, name and kind of failure that began within the last ten minutes,
// and returns how many rows it changed. A repeated failure then stays one
// row, and its first attempt keeps the time the ten minutes count from.
func BumpFailedSignIn(ctx context.Context, tx *sql.Tx, now time.Time, ip, name, how string) (int64, error) {
	res, err := tx.ExecContext(ctx, `UPDATE events SET data = json_set(data, '$.count', coalesce(json_extract(data, '$.count'), 1) + 1, '$.last_at', ?),
			updated_at = ? WHERE id = (SELECT id FROM events WHERE kind = 'sign_in_failed' AND json_extract(data, '$.ip') = ? AND json_extract(data, '$.name') = ?
			AND coalesce(json_extract(data, '$.how'), 'password') = ? AND at >= ? ORDER BY id DESC LIMIT 1)`,
		FormatTime(now), FormatTime(now), ip, name, how, FormatTime(now.Add(-10*time.Minute)))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
