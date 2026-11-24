package requests

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Remove filters for RemoveMany.
const (
	RemoveFailed   = "failed"   // every ask View.Failed says, as its card shows
	RemoveFinished = "finished" // ready in the Library, or turned down
)

// Failed says the ask was approved but will not reach the Library: no
// title was added, or the title was deleted from the Library since (its
// Problem). The Requests page shows these cards as Failed. An ask approved
// a moment ago has no title yet because adding it takes a while (Adding):
// it is not Failed until approveAddLimit has passed.
func (v View) Failed() bool {
	return v.State == domain.AskApproved && !v.Adding && (v.TitleID == nil || v.Problem != "")
}

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
// already is skipped. With a filter, ids (and byDownload) narrow it to the
// cards the page showed, and only those the filter still names go. byDownload are the
// asks the page showed Failed because their download failed: that comes
// from the Downloads view joined with the live client feed and running
// searches, which this module does not read, so they are taken by id while
// the ask is still approved. The server cannot re-check whether such a
// download has recovered since; the page sends these ids only for cards
// still shown as Failed when the owner confirms. With no ids, the filter
// names every ask it matches, plus byDownload.
func (s *Service) RemoveMany(ctx context.Context, p Person, ids []int64, filter string, byDownload ...int64) (int, error) {
	ids, err := s.removeSet(ctx, ids, filter, byDownload)
	if err != nil {
		return 0, err
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

// removeSet picks the asks RemoveMany takes (see there for the rules).
func (s *Service) removeSet(ctx context.Context, ids []int64, filter string, byDownload []int64) ([]int64, error) {
	if len(byDownload) > 0 && filter != RemoveFailed {
		return nil, errors.New("failed_downloads goes with the failed filter only")
	}
	switch filter {
	case "":
	case RemoveFailed:
		vs, err := s.Views(ctx, removeSQL2)
		if err != nil {
			return nil, err
		}
		shown := map[int64]bool{}
		for _, id := range ids {
			shown[id] = true
		}
		byDl := map[int64]bool{}
		for _, id := range byDownload {
			byDl[id] = true
		}
		all := len(ids) == 0
		ids = nil
		for _, v := range vs {
			if (v.Failed() && (all || shown[v.ID])) || byDl[v.ID] {
				ids = append(ids, v.ID)
			}
		}
	case RemoveFinished:
		rows, err := s.Store.Read().QueryContext(ctx, removeSQL3)
		if err != nil {
			return nil, err
		}
		shown := map[int64]bool{}
		for _, id := range ids {
			shown[id] = true
		}
		all := len(ids) == 0
		ids = nil
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			if all || shown[id] {
				ids = append(ids, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("filter must be failed or finished")
	}
	return ids, nil
}
