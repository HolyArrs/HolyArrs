package requests

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// ErrBadTMDBID says a TMDB id is not a positive number.
var ErrBadTMDBID = errors.New("tmdb_id must be a TMDB number")

func tmdbIDOK(id string) bool {
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strings.TrimSpace(id) == id
}

// libraryShow is the Library title of a show by TMDB id, or 0.
func (s *Service) libraryShow(ctx context.Context, tmdbID string) (int64, error) {
	var id int64
	err := s.Store.Read().QueryRowContext(ctx, libraryShowSQL, tmdbID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// FollowShowTMDB follows a show by its TMDB id. A show already in the
// Library is followed as that title; otherwise the follow waits for the
// show to be added (WatchTMDBFollows), and the person hears when it lands.
// It returns the Library title id, or 0.
func (s *Service) FollowShowTMDB(ctx context.Context, p Person, tmdbID string) (int64, error) {
	if !tmdbIDOK(tmdbID) {
		return 0, ErrBadTMDBID
	}
	id, err := s.libraryShow(ctx, tmdbID)
	if err != nil || id != 0 {
		if err == nil {
			err = s.Follow(ctx, p, id)
		}
		return id, err
	}
	if p.MaxRating != "" {
		// A Kid follows only shows their limit allows (unrated ones are refused), as when asking.
		defs, err := library.ReadShowDefaults(ctx, s.Store.Read())
		if err != nil {
			return 0, err
		}
		sh, err := shows.New(s.Library).LookupShow(ctx, "tmdb:"+tmdbID, defs.Source)
		if err != nil {
			return 0, err
		}
		if err := overLimit(p, "show", sh.Rating); err != nil {
			return 0, err
		}
	}
	now := database.FormatTime(s.now())
	return 0, s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(followShowTMDBSQL, p.UserID, tmdbID, now)
		return err
	})
}

// UnfollowShowTMDB stops a follow by TMDB id, waiting or already moved to
// the Library title.
func (s *Service) UnfollowShowTMDB(ctx context.Context, userID int64, tmdbID string) error {
	if !tmdbIDOK(tmdbID) {
		return ErrBadTMDBID
	}
	id, err := s.libraryShow(ctx, tmdbID)
	if err != nil {
		return err
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(unfollowShowTMDBSQL, userID, tmdbID); err != nil {
			return err
		}
		if id != 0 {
			_, err := tx.Exec(unfollowShowTMDBSQL2, userID, id)
			return err
		}
		return nil
	})
}

// IsFollowingShowTMDB says whether a person follows a show by TMDB id.
func (s *Service) IsFollowingShowTMDB(ctx context.Context, userID int64, tmdbID string) (bool, error) {
	if !tmdbIDOK(tmdbID) {
		return false, ErrBadTMDBID
	}
	var n int
	err := s.Store.Read().QueryRowContext(ctx, isFollowingShowTMDBSQL,
		userID, tmdbID, userID, tmdbID).Scan(&n)
	return n > 0, err
}

// libraryMovie is the Library title of a movie by TMDB id, or 0.
func (s *Service) libraryMovie(ctx context.Context, tmdbID string) (int64, error) {
	var id int64
	err := s.Store.Read().QueryRowContext(ctx, libraryMovieSQL, tmdbID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// FollowMovieTMDB follows a movie by its TMDB id. A movie in the Library
// is followed as that title; otherwise the follow waits for the movie
// (often one not out yet), follows the Library title once it is added,
// and the person hears once its file is in. A Kid follows only movies their limit
// allows, as when asking.
func (s *Service) FollowMovieTMDB(ctx context.Context, p Person, tmdbID string) error {
	if !tmdbIDOK(tmdbID) {
		return ErrBadTMDBID
	}
	// The limit first, so an over-limit movie answers the same whether or not it is in the Library.
	if p.MaxRating != "" {
		m, err := s.Metadata.Movie(ctx, "tmdb:"+tmdbID)
		if err != nil {
			return err
		}
		if err := overLimit(p, "movie", m.Rating); err != nil {
			return err
		}
	}
	id, err := s.libraryMovie(ctx, tmdbID)
	if err != nil {
		return err
	}
	if id != 0 {
		return s.FollowTitle(ctx, p, id)
	}
	now := database.FormatTime(s.now())
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(followMovieTMDBSQL, p.UserID, tmdbID, now)
		return err
	})
}

// UnfollowMovieTMDB stops a follow of a movie by TMDB id, waiting or of
// its Library title.
func (s *Service) UnfollowMovieTMDB(ctx context.Context, userID int64, tmdbID string) error {
	if !tmdbIDOK(tmdbID) {
		return ErrBadTMDBID
	}
	id, err := s.libraryMovie(ctx, tmdbID)
	if err != nil {
		return err
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(unfollowMovieTMDBSQL, userID, tmdbID); err != nil {
			return err
		}
		if id != 0 {
			_, err := tx.Exec(unfollowMovieTMDBSQL2, userID, id)
			return err
		}
		return nil
	})
}

// IsFollowingMovieTMDB says whether a person follows a movie by TMDB id,
// waiting for it or as its Library title.
func (s *Service) IsFollowingMovieTMDB(ctx context.Context, userID int64, tmdbID string) (bool, error) {
	if !tmdbIDOK(tmdbID) {
		return false, ErrBadTMDBID
	}
	var n int
	err := s.Store.Read().QueryRowContext(ctx, isFollowingMovieTMDBSQL,
		userID, tmdbID, userID, tmdbID).Scan(&n)
	return n > 0, err
}

// landMovieFollows moves the follows of the people waiting for a
// just-added movie onto its Library title. They are told once its file is
// in (TellMovieFollows), not at the add: the add only means HolyArrs will
// look for it. Kids whose limit does not allow it are dropped without a
// notice. It returns how many follows it moved.
func (s *Service) landMovieFollows(ctx context.Context, titleID int64, tmdbID string) (int, error) {
	rows, err := s.Store.Read().QueryContext(ctx, landMovieFollowsSQL, tmdbID)
	if err != nil {
		return 0, err
	}
	var ids []int64
	var maxes []string
	for rows.Next() {
		var id int64
		var max string
		if err := rows.Scan(&id, &max); err != nil {
			rows.Close()
			return 0, err
		}
		ids, maxes = append(ids, id), append(maxes, max)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	keep := map[int64]bool{}
	for i, id := range ids {
		if maxes[i] != "" {
			switch err := libraryOverLimit(ctx, s.Store.Read(), Person{UserID: id, MaxRating: maxes[i]}, "movie", titleID); {
			case errors.Is(err, ErrAboveLimit):
				continue
			case err != nil:
				return 0, err
			}
		}
		keep[id] = true
	}
	now := database.FormatTime(s.now())
	moved := 0
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		moved = 0
		for _, id := range ids {
			if !keep[id] {
				// Dropped: no follow and no notice later.
				if _, err := tx.Exec(landMovieFollowsSQL2, id, tmdbID); err != nil {
					return err
				}
				continue
			}
			// The waiting row stays until the file is in: it says who is still to be told.
			res, err := tx.Exec(landMovieFollowsSQL3, id, titleID, now)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				moved++
			}
		}
		return nil
	})
	return moved, err
}

// TellMovieFollows tells the people who followed a movie before its file
// was in (before it was in the Library, or in it without a file) that it is ready to watch, once its file is in (an in-app
// message and the Ready alert), once each. People who stopped following
// it in the meantime are not told. It returns how many people it told.
func (s *Service) TellMovieFollows(ctx context.Context) (int, error) {
	rows, err := s.Store.Read().QueryContext(ctx, tellMovieFollowsSQL)
	if err != nil {
		return 0, err
	}
	type movie struct {
		id           int64
		name, tmdbID string
		people       []person
	}
	var ms []*movie
	for rows.Next() {
		var id int64
		var name, tmdbID string
		var p person
		if err := rows.Scan(&id, &name, &tmdbID, &p.ID, &p.Name, &p.Email, &p.DiscordID); err != nil {
			rows.Close()
			return 0, err
		}
		if len(ms) == 0 || ms[len(ms)-1].id != id {
			ms = append(ms, &movie{id: id, name: name, tmdbID: tmdbID})
		}
		p.Name = strings.TrimSpace(p.Name)
		ms[len(ms)-1].people = append(ms[len(ms)-1].people, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	told := 0
	for _, m := range ms {
		now := database.FormatTime(s.now())
		text := m.name + " is in the Library"
		var tell []person
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			tell = tell[:0]
			for _, p := range m.people {
				// Only the check that removes the waiting row tells (two checks can run at once).
				res, err := tx.Exec(tellMovieFollowsSQL2, p.ID, m.tmdbID)
				if err != nil {
					return err
				}
				if n, _ := res.RowsAffected(); n == 0 {
					continue
				}
				if _, err := tx.Exec(tellMovieFollowsSQL3, now, p.ID, text+" and ready to watch.", m.id); err != nil {
					return err
				}
				tell = append(tell, p)
			}
			return nil
		})
		if err != nil {
			return told, err
		}
		if len(tell) == 0 {
			continue
		}
		told += len(tell)
		msg := alerts.Message{Moment: alerts.MomentReady, Subject: text, Text: text + " and ready to watch. You followed it, so here it is."}
		for _, p := range tell {
			msg.People = append(msg.People, alerts.Person{Name: p.Name, Email: p.Email, DiscordID: p.DiscordID})
		}
		s.send(ctx, msg)
	}
	return told, nil
}

// WatchTMDBFollows moves the waiting follows of a show onto its Library
// title once it is added, and tells those people it landed. From then on
// they hear about its new episodes and seasons like any follower.
func (s *Service) WatchTMDBFollows() {
	s.Store.OnEvent(func(e domain.Event) {
		if e.Kind != domain.EventTitleAdded || e.TitleID == nil {
			return
		}
		id := *e.TitleID
		s.sending.Add(1)
		go func() {
			defer s.sending.Done()
			jobs.Guard("tmdb_follows", func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if _, err := s.LandTMDBFollows(ctx, id); err != nil {
					log.Printf("asks: follows by TMDB id: %v", err)
				}
			})
		}()
	})
}

// LandTMDBFollows moves the waiting follows of a just-added show to the
// title and sends each person a notice, and returns how many were told.
// For a movie it moves the follows only (they hear once its file is in,
// TellMovieFollows) and returns how many follows it moved.
func (s *Service) LandTMDBFollows(ctx context.Context, titleID int64) (int, error) {
	var name, tmdbID, kind string
	err := s.Store.Read().QueryRowContext(ctx, landTMDBFollowsSQL, titleID).Scan(&name, &tmdbID, &kind)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && tmdbID == "") {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if kind == "movie" {
		return s.landMovieFollows(ctx, titleID, tmdbID)
	}
	rows, err := s.Store.Read().QueryContext(ctx, landTMDBFollowsSQL2, tmdbID)
	if err != nil {
		return 0, err
	}
	var all []int64
	var tell []person
	var limits = map[int64]string{}
	for rows.Next() {
		var p person
		var on int
		var max string
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.DiscordID, &on, &max); err != nil {
			rows.Close()
			return 0, err
		}
		if max != "" {
			limits[p.ID] = max
			continue
		}
		all = append(all, p.ID)
		if on == 1 {
			p.Name = strings.TrimSpace(p.Name)
			tell = append(tell, p)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	// Followers with a rating limit move over only when the title is within
	// it (as Follow checks); the others are dropped, with no notice.
	for uid, max := range limits {
		switch err := libraryOverLimit(ctx, s.Store.Read(), Person{UserID: uid, MaxRating: max}, "show", titleID); {
		case err == nil:
			all = append(all, uid)
		case errors.Is(err, ErrAboveLimit):
		default:
			return 0, err
		}
	}
	if len(limits) > 0 {
		tell = tell[:0:0]
		in := map[int64]bool{}
		for _, uid := range all {
			in[uid] = true
		}
		rows, err := s.Store.Read().QueryContext(ctx, landTMDBFollowsSQL3, tmdbID)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var p person
			if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.DiscordID); err != nil {
				rows.Close()
				return 0, err
			}
			if in[p.ID] {
				p.Name = strings.TrimSpace(p.Name)
				tell = append(tell, p)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, err
		}
	}
	if len(all) == 0 {
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(landTMDBFollowsSQL4, tmdbID)
			return err
		})
		return 0, nil
	}
	now := database.FormatTime(s.now())
	text := name + " is in the Library"
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, uid := range all {
			if _, err := tx.Exec(landTMDBFollowsSQL5, uid, titleID, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(landTMDBFollowsSQL6, tmdbID); err != nil {
			return err
		}
		for _, p := range tell {
			if _, err := tx.Exec(landTMDBFollowsSQL7, now, p.ID, text+".", titleID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil || len(tell) == 0 {
		return 0, err
	}
	m := alerts.Message{Moment: alerts.MomentNewEpisodes, Subject: text, Text: text + ". You will hear about its new episodes and seasons."}
	for _, p := range tell {
		m.People = append(m.People, alerts.Person{Name: p.Name, Email: p.Email, DiscordID: p.DiscordID})
	}
	s.send(ctx, m)
	return len(tell), nil
}
