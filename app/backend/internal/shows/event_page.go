package shows

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// Queryer is what the event page reads with (*sql.DB fits; tests count calls).
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// EventPage holds what a page of Events names, read with one query per
// kind of row (titles, Slots, Items, Versions) instead of one per Event.
type EventPage struct {
	titles   map[int64]pageTitle
	slots    map[int64]pageSlot
	items    map[int64]downloads.EpisodeLine
	versions map[int64]pageVersion
}

type pageTitle struct {
	name, kind string
	versions   int
}

type pageSlot struct {
	line    downloads.EpisodeLine
	isTrack bool
	label   string // the Slot's Version
}

type pageVersion struct {
	title int64
	label string
}

// idChunk keeps an IN list under SQLite's variable limit.
const idChunk = 500

// eachIn runs query (with one "%s" for the IN list) over ids in chunks.
func eachIn(ctx context.Context, db Queryer, query string, ids []int64, scan func(*sql.Rows) error) error {
	for start := 0; start < len(ids); start += idChunk {
		part := ids[start:min(start+idChunk, len(ids))]
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf(query, strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")), args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			if err := scan(rows); err != nil {
				_ = rows.Close()
				return err
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

// eventSlots are the Slots an Event names: the Slots an import placed, a
// pack's Slots, or the Event's own Slot.
func eventSlots(e domain.Event) []int64 {
	var ids []int64
	for _, k := range []string{"placed_slot_ids", "slot_ids"} {
		if raw, ok := e.Data[k].([]any); ok && len(raw) > 0 {
			for _, x := range raw {
				if f, ok := x.(float64); ok {
					ids = append(ids, int64(f))
				}
			}
			break
		}
	}
	if len(ids) == 0 && e.SlotID != nil {
		ids = []int64{*e.SlotID}
	}
	return ids
}

// eventItems are the episodes a delete Event names.
func eventItems(e domain.Event) []int64 {
	var items []int64
	if raw, ok := e.Data["item_ids"].([]any); ok {
		for _, x := range raw {
			if f, ok := x.(float64); ok {
				items = append(items, int64(f))
			}
		}
	} else if f, ok := e.Data["item_id"].(float64); ok {
		items = []int64{int64(f)}
	}
	return items
}

// LoadEventPage reads, in a few queries, the titles, Slots, Items and
// Versions a page of Events names.
func LoadEventPage(ctx context.Context, db Queryer, es []domain.Event) (*EventPage, error) {
	p := &EventPage{titles: map[int64]pageTitle{}, slots: map[int64]pageSlot{}, items: map[int64]downloads.EpisodeLine{}, versions: map[int64]pageVersion{}}
	seen := map[string]map[int64]bool{"t": {}, "s": {}, "i": {}, "v": {}}
	var titles, slots, items, versions []int64
	add := func(kind string, list *[]int64, id int64) {
		if !seen[kind][id] {
			seen[kind][id] = true
			*list = append(*list, id)
		}
	}
	for _, e := range es {
		if e.TitleID == nil {
			continue
		}
		add("t", &titles, *e.TitleID)
		for _, id := range eventSlots(e) {
			add("s", &slots, id)
		}
		if e.SlotID != nil {
			add("s", &slots, *e.SlotID)
		}
		for _, id := range eventItems(e) {
			add("i", &items, id)
		}
		if f, ok := e.Data["version_id"].(float64); ok {
			add("v", &versions, int64(f))
		}
	}
	err := eachIn(ctx, db, `SELECT t.id, t.name, t.kind, (SELECT count(*) FROM versions v WHERE v.title_id = t.id) FROM titles t WHERE t.id IN (%s)`, titles, func(r *sql.Rows) error {
		var id int64
		var t pageTitle
		if err := r.Scan(&id, &t.name, &t.kind, &t.versions); err != nil {
			return err
		}
		p.titles[id] = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = eachIn(ctx, db, `SELECT s.id, s.state, i.kind, CASE i.kind WHEN 'track' THEN COALESCE(json_extract(i.numbers, '$.disc'), 1) ELSE json_extract(i.numbers, '$.season') END,
		CASE i.kind WHEN 'track' THEN json_extract(i.numbers, '$.track') ELSE json_extract(i.numbers, '$.episode') END, i.name, v.label
		FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id WHERE s.id IN (%s)`, slots, func(r *sql.Rows) error {
		var s pageSlot
		var kind string
		var season, episode sql.NullInt64
		var name sql.NullString
		if err := r.Scan(&s.line.SlotID, &s.line.State, &kind, &season, &episode, &name, &s.label); err != nil {
			return err
		}
		s.line.Season, s.line.Episode, s.line.Name = int(season.Int64), int(episode.Int64), name.String
		s.line.Label = fmt.Sprintf("S%02dE%02d", s.line.Season, s.line.Episode)
		if kind == "track" {
			s.isTrack, s.line.Track = true, true
			s.line.Label = fmt.Sprintf("Track %02d", s.line.Episode)
			if s.line.Season > 1 {
				s.line.Label = fmt.Sprintf("Disc %d track %02d", s.line.Season, s.line.Episode)
			}
		}
		p.slots[s.line.SlotID] = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = eachIn(ctx, db, `SELECT id, json_extract(numbers, '$.season'), json_extract(numbers, '$.episode'), name FROM items WHERE id IN (%s)`, items, func(r *sql.Rows) error {
		var id int64
		var se, ep sql.NullInt64
		var name sql.NullString
		if err := r.Scan(&id, &se, &ep, &name); err != nil {
			return err
		}
		p.items[id] = downloads.EpisodeLine{Season: int(se.Int64), Episode: int(ep.Int64), Label: fmt.Sprintf("S%02dE%02d", se.Int64, ep.Int64), Name: name.String}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = eachIn(ctx, db, `SELECT id, title_id, label FROM versions WHERE id IN (%s)`, versions, func(r *sql.Rows) error {
		var id int64
		var v pageVersion
		if err := r.Scan(&id, &v.title, &v.label); err != nil {
			return err
		}
		p.versions[id] = v
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// AppendEvent is es with e added when it is not nil (a copy; es stays).
func AppendEvent(es []domain.Event, e *domain.Event) []domain.Event {
	if e == nil {
		return es
	}
	return append(append(make([]domain.Event, 0, len(es)+1), es...), *e)
}

// TitleName is the name of an Event's title ("" when gone).
func (p *EventPage) TitleName(e domain.Event) string {
	if e.TitleID == nil {
		return ""
	}
	return p.titles[*e.TitleID].name
}

// Episode names the episodes a show Event is about, as the Downloads
// screen does ("S01E02 The Mill", "S01E01-E03, 3 episodes"): the Slots an
// import placed, a pack's Slots, the Event's own Slot, or the episodes a
// delete names. A movie Event names none.
func (p *EventPage) Episode(e domain.Event) string {
	if e.TitleID == nil || p.titles[*e.TitleID].kind != "show" {
		return ""
	}
	if ids := eventSlots(e); len(ids) > 0 {
		eps := []downloads.EpisodeLine{}
		for _, id := range ids {
			if s, ok := p.slots[id]; ok {
				eps = append(eps, s.line)
			}
		}
		downloads.SortLines(eps)
		return downloads.Covers(eps)
	}
	var eps []downloads.EpisodeLine
	for _, id := range eventItems(e) {
		if l, ok := p.items[id]; ok {
			eps = append(eps, l)
		}
	}
	return downloads.Covers(eps)
}

// Version names the Version an Event is about (its Slot's, the first of
// the Slots it names, or its version_id), only when the title has more
// than one Version.
func (p *EventPage) Version(e domain.Event) string {
	if e.TitleID == nil || p.titles[*e.TitleID].versions < 2 {
		return ""
	}
	if f, ok := e.Data["version_id"].(float64); ok {
		if v, ok := p.versions[int64(f)]; ok && v.title == *e.TitleID {
			return v.label
		}
		return ""
	}
	if e.SlotID != nil {
		return p.slots[*e.SlotID].label
	}
	if ids := eventSlots(e); len(ids) > 0 {
		return p.slots[ids[0]].label
	}
	return ""
}
