package requests

import (
	"context"
	"database/sql"
	"log"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
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
	if titleID <= 0 {
		return 0, nil
	}
	return s.relink(ctx, titleID)
}

// RelinkAll does RelinkTitle for every Library title, once at start, so
// asks left without their title before this existed find it again.
func (s *Service) RelinkAll(ctx context.Context) (int, error) {
	return s.relink(ctx, 0)
}

// relinkAsk is an ask that may follow a Library title back.
type relinkAsk struct {
	kind, tmdb, tvdb string
	askID            int64
	titleID          sql.NullInt64
}

func (s *Service) relink(ctx context.Context, titleID int64) (int, error) {
	db := s.Store.Read()
	titles, err := mediafiles.LiveTitleIDs(ctx, db, titleID)
	if err != nil || len(titles) == 0 {
		return 0, err
	}
	removed, err := mediafiles.RemovedTitleIDs(ctx, db)
	if err != nil {
		return 0, err
	}
	asks, err := lostAsks(ctx, db, removed)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range asks {
		// the first Library title with that id wins
		i := slices.IndexFunc(titles, func(t mediafiles.TitleIDs) bool { return sameTitle(a, t) })
		if i < 0 {
			continue
		}
		to := titles[i].ID
		var moved int64
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec(relinkSetSQL, to, database.FormatTime(s.now()), a.askID, a.titleID)
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
		if err := s.followAskers(ctx, a.askID, to); err != nil {
			log.Printf("asks: follows of ask %d: %v", a.askID, err)
		}
		s.changed(ctx, a.askID)
	}
	return n, nil
}

// lostAsks lists the approved or ready movie and show asks whose title was
// never linked or was deleted from the Library, by id.
func lostAsks(ctx context.Context, db database.Lister, removed map[int64]bool) ([]relinkAsk, error) {
	rows, err := db.QueryContext(ctx, relinkAsksSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []relinkAsk
	for rows.Next() {
		var a relinkAsk
		if err := rows.Scan(&a.askID, &a.kind, &a.tmdb, &a.tvdb, &a.titleID); err != nil {
			return nil, err
		}
		if !a.titleID.Valid || removed[a.titleID.Int64] {
			out = append(out, a)
		}
	}
	return out, rows.Err()
}

func sameTitle(a relinkAsk, t mediafiles.TitleIDs) bool {
	if t.Kind != a.kind {
		return false
	}
	return (a.tmdb != "" && t.TMDB == a.tmdb) || (a.kind == "show" && a.tvdb != "" && t.TVDB == a.tvdb)
}
