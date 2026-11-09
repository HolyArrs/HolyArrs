package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// movieFromTitle rebuilds the name facts a release name needs.
func movieFromTitle(t mediafiles.Title) metadata.Movie {
	m := metadata.Movie{Name: t.Name}
	if t.Year != nil {
		m.Year = *t.Year
	}
	return m
}

// historyTimes spreads what this run made over the last weeks, so History,
// Recently added and the Downloads list read like a library in use. Only
// display times change (when an Event happened, when a row was made, when
// a download ran); no state, link or count does.
func (w *world) historyTimes(ctx context.Context) error {
	if err := checkMarker(w.o.ConfigDir); err != nil {
		return err
	}
	now := time.Now().UTC()
	// Titles this run added: added between 35 and 21 days ago, in the
	// order added, with their own Events.
	if w.addSpan != nil {
		ids := []int64{}
		for _, id := range w.m.Titles {
			ids = append(ids, id)
		}
		var lo, hi int64
		_ = w.s.Read().QueryRowContext(ctx, "SELECT COALESCE(min(id), 0), COALESCE(max(id), 0) FROM titles WHERE id > ?", w.addSpan.titles).Scan(&lo, &hi)
		if hi > 0 {
			start := now.Add(-35 * 24 * time.Hour)
			span := 14 * 24 * time.Hour
			n := hi - lo + 1
			err := w.s.Write(ctx, func(tx *sql.Tx) error {
				for id := lo; id <= hi; id++ {
					at := database.FormatTime(start.Add(time.Duration(id-lo) * span / time.Duration(n)))
					if _, err := tx.Exec("UPDATE titles SET created_at = ? WHERE id = ?", at, id); err != nil {
						return err
					}
					if _, err := tx.Exec("UPDATE files SET added_at = ?, created_at = ? WHERE origin = 'adopted' AND version_id IN (SELECT id FROM versions WHERE title_id = ?)", at, at, id); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			err = w.s.WriteEvents(ctx, func(tx *sql.Tx) error {
				for id := lo; id <= hi; id++ {
					at := start.Add(time.Duration(id-lo) * span / time.Duration(n))
					if _, err := tx.Exec(`UPDATE events SET at = strftime('%Y-%m-%dT%H:%M:%fZ', ?, '+' || (id % 50) || ' seconds'), created_at = ?
						WHERE title_id = ? AND id > ? AND id <= ?`, database.FormatTime(at), database.FormatTime(at), id, w.addSpan.events, w.addSpanEnd.events); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	for _, st := range w.stamps {
		at := func(i int64) string { return database.FormatTime(st.at.Add(time.Duration(i) * st.step)) }
		err := w.s.WriteEvents(ctx, func(tx *sql.Tx) error {
			rows, err := tx.Query("SELECT id FROM events WHERE id > ? AND id <= ? ORDER BY id", st.from.events, st.to.events)
			if err != nil {
				return err
			}
			var ids []int64
			for rows.Next() {
				var id int64
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
			for i, id := range ids {
				if _, err := tx.Exec("UPDATE events SET at = ?, created_at = ?, updated_at = ? WHERE id = ?", at(int64(i)), at(int64(i)), at(int64(i)), id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		start, end := at(0), at(3)
		err = w.s.Write(ctx, func(tx *sql.Tx) error {
			for _, q := range []struct {
				sql  string
				a, b int64
			}{
				{"UPDATE releases SET created_at = ?, published_at = strftime('%Y-%m-%dT%H:%M:%fZ', ?, '-5 hours') WHERE id > ? AND id <= ?", st.from.releases, st.to.releases},
				{"UPDATE downloads SET created_at = ?, started_at = ?, progress_at = ? WHERE id > ? AND id <= ?", st.from.downloads, st.to.downloads},
				{"UPDATE downloads SET finished_at = ?, updated_at = ? WHERE finished_at IS NOT NULL AND id > ? AND id <= ?", st.from.downloads, st.to.downloads},
				{"UPDATE imports SET created_at = ?, updated_at = ? WHERE id > ? AND id <= ?", st.from.imports, st.to.imports},
				{"UPDATE files SET added_at = ?, created_at = ? WHERE id > ? AND id <= ?", st.from.files, st.to.files},
			} {
				var args []any
				switch q.sql[:24] {
				case "UPDATE releases SET crea":
					args = []any{start, start, q.a, q.b}
				case "UPDATE downloads SET cre":
					args = []any{start, start, start, q.a, q.b}
				default:
					args = []any{end, end, q.a, q.b}
				}
				if _, err := tx.Exec(q.sql, args...); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	w.say("   %d activity spans and the added titles moved into the last five weeks", len(w.stamps))
	return nil
}
