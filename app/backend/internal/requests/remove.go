package requests

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Remove filters for RemoveMany.
const (
	RemoveFailed   = "failed"   // approved, and no title was ever added
	RemoveFinished = "finished" // ready in the Library, or turned down
)

// Remove deletes an ask in any state, for a person who approves. Only the
// request goes: its title, files, downloads and follows stay, and everyone
// who asked simply no longer sees it.
func (s *Service) Remove(ctx context.Context, id int64, p Person) error {
	a, err := get(ctx, s.Store.Read(), id)
	if err != nil {
		return err
	}
	var ids []int64
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		rows, err := tx.Query(withdrawSQL2, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var u int64
			if err := rows.Scan(&u); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, u)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		res, err := tx.Exec(removeSQL, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return database.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskWithdrawn, Summary: p.Name + " removed the request for " + label(a),
		Data: map[string]any{"ask_id": a.ID}, GroupKey: groupKey(a.ID)})
	if s.Live != nil {
		s.Live.PublishApprovers("ask_removed", map[string]any{"id": id})
		for _, u := range ids {
			s.Live.PublishUser("ask_removed", map[string]any{"id": id}, u)
		}
	}
	return nil
}

// RemoveMany removes the asks in ids, or every ask the filter names
// ("failed" or "finished"), and says how many went. An id that is gone
// already is skipped.
func (s *Service) RemoveMany(ctx context.Context, p Person, ids []int64, filter string) (int, error) {
	switch filter {
	case "":
	case RemoveFailed, RemoveFinished:
		q := removeSQL2
		if filter == RemoveFinished {
			q = removeSQL3
		}
		rows, err := s.Store.Read().QueryContext(ctx, q)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return 0, err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, err
		}
	default:
		return 0, errors.New("filter must be failed or finished")
	}
	n := 0
	for _, id := range ids {
		switch err := s.Remove(ctx, id, p); {
		case err == nil:
			n++
		case errors.Is(err, database.ErrNotFound):
		default:
			return n, errors.New("could not remove request " + strconv.FormatInt(id, 10) + ": " + err.Error())
		}
	}
	return n, nil
}
