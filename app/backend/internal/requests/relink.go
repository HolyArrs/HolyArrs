package requests

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// WatchRelink links old asks to a title as it comes into the Library: a
// movie or show deleted after it was approved and added back later is the
// same title again, so its asks follow it instead of reading as failed.
// It also runs RelinkAll once in the background for asks left behind
// before this watch existed.
func (s *Service) WatchRelink() {
	s.sending.Add(1)
	go func() {
		defer s.sending.Done()
		jobs.Guard("ask_relink", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if _, err := s.RelinkAll(ctx); err != nil {
				log.Printf("asks: relink on start: %v", err)
			}
		})
	}()
	s.Store.OnEvent(func(e domain.Event) {
		if e.Kind != domain.EventTitleAdded || e.TitleID == nil {
			return
		}
		id := *e.TitleID
		s.sending.Add(1)
		go func() {
			defer s.sending.Done()
			jobs.Guard("ask_relink", func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if _, err := s.RelinkTitle(ctx, id); err != nil {
					log.Printf("asks: relink title %d: %v", id, err)
				}
			})
		}()
	})
}

// RelinkTitle links the approved or ready asks for the same title as
// titleID (same kind and TMDB id, or TVDB id for a show) whose own title
// is gone. It returns how many it linked.
func (s *Service) RelinkTitle(ctx context.Context, titleID int64) (int, error) {
	return s.relink(ctx, relinkSQL+relinkTitleSQL+relinkOrderSQL, titleID)
}

// RelinkAll does RelinkTitle for every Library title, once at start, so
// asks left without their title before this existed find it again.
func (s *Service) RelinkAll(ctx context.Context) (int, error) {
	return s.relink(ctx, relinkSQL+relinkOrderSQL)
}

func (s *Service) relink(ctx context.Context, q string, args ...any) (int, error) {
	rows, err := s.Store.Read().QueryContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	pairs := map[int64]int64{}
	var order []int64
	for rows.Next() {
		var ask, title int64
		if err := rows.Scan(&ask, &title); err != nil {
			rows.Close()
			return 0, err
		}
		if _, ok := pairs[ask]; ok {
			continue // the first Library title with that id wins
		}
		pairs[ask] = title
		order = append(order, ask)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, ask := range order {
		var moved int64
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec(relinkSetSQL, pairs[ask], database.FormatTime(s.now()), ask)
			if err == nil {
				moved, _ = res.RowsAffected()
			}
			return err
		})
		if err != nil {
			return n, err
		}
		if moved == 0 {
			continue
		}
		n++
		if err := s.followAskers(ctx, ask, pairs[ask]); err != nil {
			log.Printf("asks: follows of ask %d: %v", ask, err)
		}
		s.changed(ctx, ask)
	}
	return n, nil
}
