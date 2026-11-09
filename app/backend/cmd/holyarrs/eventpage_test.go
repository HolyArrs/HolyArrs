package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

type countingDB struct {
	db *sql.DB
	n  int
}

func (c *countingDB) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	c.n++
	return c.db.QueryContext(ctx, q, args...)
}

// 4.8.11 review: a page of History rows is named with one query per kind
// of row (titles, Slots, Items, Versions), however many rows it has.
func TestEventPageQueriesDoNotGrowWithRows(t *testing.T) {
	t.Parallel()
	srv, s, _, _ := showSetup(t)
	var add struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/shows", `{"id":"tmdb:100","more_versions":[{"label":"4K"}]}`, 201), &add)
	rows, err := s.Read().Query("SELECT s.id, s.item_id, s.version_id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?", add.Title.ID)
	if err != nil {
		t.Fatal(err)
	}
	var es []domain.Event
	for rows.Next() {
		var slot, item, version int64
		if err := rows.Scan(&slot, &item, &version); err != nil {
			t.Fatal(err)
		}
		id := add.Title.ID
		es = append(es,
			domain.Event{TitleID: &id, SlotID: &slot, Data: map[string]any{}},
			domain.Event{TitleID: &id, Data: map[string]any{"slot_ids": []any{float64(slot)}}},
			domain.Event{TitleID: &id, Data: map[string]any{"item_id": float64(item)}},
			domain.Event{TitleID: &id, Data: map[string]any{"version_id": float64(version)}})
	}
	_ = rows.Close()
	if len(es) < 8 {
		t.Fatalf("only %d events", len(es))
	}
	count := func(list []domain.Event) (int, *shows.EventPage) {
		c := &countingDB{db: s.Read()}
		p, err := shows.LoadEventPage(context.Background(), c, list)
		if err != nil {
			t.Fatal(err)
		}
		return c.n, p
	}
	one, _ := count(es[:4])
	all, p := count(es)
	if one != 4 || all != 4 {
		t.Fatalf("queries: %d for 4 rows, %d for %d rows", one, all, len(es))
	}
	if p.Episode(es[0]) == "" || p.Version(es[0]) == "" || p.Episode(es[2]) == "" || p.Version(es[3]) == "" {
		t.Fatalf("names: %q %q %q %q", p.Episode(es[0]), p.Version(es[0]), p.Episode(es[2]), p.Version(es[3]))
	}
}
