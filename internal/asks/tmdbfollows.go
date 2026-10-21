package asks

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/alerts"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
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
	err := s.Store.Read().QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = 'show' AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?", tmdbID).Scan(&id)
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
		sh, err := s.Library.LookupShow(ctx, "tmdb:"+tmdbID, defs.Source)
		if err != nil {
			return 0, err
		}
		if err := overLimit(p, "show", sh.Rating); err != nil {
			return 0, err
		}
	}
	now := store.FormatTime(s.now())
	return 0, s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO tmdb_follows (user_id, kind, tmdb_id, created_at) VALUES (?, 'show', ?, ?) ON CONFLICT DO NOTHING", p.UserID, tmdbID, now)
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
		if _, err := tx.Exec("DELETE FROM tmdb_follows WHERE user_id = ? AND kind = 'show' AND tmdb_id = ?", userID, tmdbID); err != nil {
			return err
		}
		if id != 0 {
			_, err := tx.Exec("DELETE FROM follows WHERE user_id = ? AND title_id = ?", userID, id)
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
	err := s.Store.Read().QueryRowContext(ctx, `SELECT (SELECT count(*) FROM tmdb_follows WHERE user_id = ? AND kind = 'show' AND tmdb_id = ?)
		+ (SELECT count(*) FROM follows f JOIN titles t ON t.id = f.title_id WHERE f.user_id = ? AND t.kind = 'show' AND t.state != 'removed' AND json_extract(t.source_ids, '$.tmdb') = ?)`,
		userID, tmdbID, userID, tmdbID).Scan(&n)
	return n > 0, err
}

// ErrInLibrary says a movie is in the Library already, so there is nothing to wait for.
var ErrInLibrary = errors.New("the movie is in the Library already")

// libraryMovie is the Library title of a movie by TMDB id, or 0.
func (s *Service) libraryMovie(ctx context.Context, tmdbID string) (int64, error) {
	var id int64
	err := s.Store.Read().QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = 'movie' AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?", tmdbID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// FollowMovieTMDB follows a movie that is not in the Library yet (often
// one not out yet) by its TMDB id; the person hears once when it lands.
// A Kid follows only movies their limit allows, as when asking.
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
	if id, err := s.libraryMovie(ctx, tmdbID); err != nil || id != 0 {
		if err == nil {
			err = ErrInLibrary
		}
		return err
	}
	now := store.FormatTime(s.now())
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO tmdb_movie_follows (user_id, tmdb_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", p.UserID, tmdbID, now)
		return err
	})
}

// UnfollowMovieTMDB stops waiting for a movie.
func (s *Service) UnfollowMovieTMDB(ctx context.Context, userID int64, tmdbID string) error {
	if !tmdbIDOK(tmdbID) {
		return ErrBadTMDBID
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM tmdb_movie_follows WHERE user_id = ? AND tmdb_id = ?", userID, tmdbID)
		return err
	})
}

// IsFollowingMovieTMDB says whether a person waits for a movie by TMDB id.
func (s *Service) IsFollowingMovieTMDB(ctx context.Context, userID int64, tmdbID string) (bool, error) {
	if !tmdbIDOK(tmdbID) {
		return false, ErrBadTMDBID
	}
	var n int
	err := s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM tmdb_movie_follows WHERE user_id = ? AND tmdb_id = ?", userID, tmdbID).Scan(&n)
	return n > 0, err
}

// landMovieFollows tells the people waiting for a just-added movie that it
// is in the Library (an in-app message and the Ready alert) and ends their
// follows. Kids whose limit does not allow it are dropped without a notice.
func (s *Service) landMovieFollows(ctx context.Context, titleID int64, name, tmdbID string) (int, error) {
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, ''), u.max_rating
		FROM tmdb_movie_follows f JOIN users u ON u.id = f.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		WHERE f.tmdb_id = ? AND u.state != 'disabled' ORDER BY u.id`, tmdbID)
	if err != nil {
		return 0, err
	}
	var tell []person
	var maxes []string
	for rows.Next() {
		var p person
		var max string
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.DiscordID, &max); err != nil {
			rows.Close()
			return 0, err
		}
		p.Name = strings.TrimSpace(p.Name)
		tell, maxes = append(tell, p), append(maxes, max)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	kept := tell[:0]
	for i, p := range tell {
		if maxes[i] != "" {
			switch err := libraryOverLimit(ctx, s.Store.Read(), Person{UserID: p.ID, MaxRating: maxes[i]}, "movie", titleID); {
			case errors.Is(err, ErrAboveLimit):
				continue
			case err != nil:
				return 0, err
			}
		}
		kept = append(kept, p)
	}
	tell = kept
	now := store.FormatTime(s.now())
	text := name + " is in the Library"
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM tmdb_movie_follows WHERE tmdb_id = ?", tmdbID); err != nil {
			return err
		}
		for _, p := range tell {
			if _, err := tx.Exec("INSERT INTO messages (created_at, user_id, kind, text, title_id) VALUES (?, ?, 'ready', ?, ?)", now, p.ID, text+".", titleID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil || len(tell) == 0 {
		return 0, err
	}
	m := alerts.Message{Moment: alerts.MomentReady, Subject: text, Text: text + ". You followed it, so here it is."}
	for _, p := range tell {
		m.People = append(m.People, alerts.Person{Name: p.Name, Email: p.Email, DiscordID: p.DiscordID})
	}
	s.send(ctx, m)
	return len(tell), nil
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
			supervisor.Guard("tmdb_follows", func() {
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
// title and sends each person a notice; for a movie it tells the people
// waiting for it. It returns how many were told.
func (s *Service) LandTMDBFollows(ctx context.Context, titleID int64) (int, error) {
	var name, tmdbID, kind string
	err := s.Store.Read().QueryRowContext(ctx, "SELECT name, coalesce(json_extract(source_ids, '$.tmdb'), ''), kind FROM titles WHERE id = ? AND kind IN ('show', 'movie')", titleID).Scan(&name, &tmdbID, &kind)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && tmdbID == "") {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if kind == "movie" {
		return s.landMovieFollows(ctx, titleID, name, tmdbID)
	}
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, ''), coalesce(x.new_episodes, 1), u.max_rating
		FROM tmdb_follows f JOIN users u ON u.id = f.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		WHERE f.kind = 'show' AND f.tmdb_id = ? AND u.state != 'disabled' ORDER BY u.id`, tmdbID)
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
		rows, err := s.Store.Read().QueryContext(ctx, `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, '') FROM users u
			LEFT JOIN user_ask_settings x ON x.user_id = u.id WHERE u.state != 'disabled' AND coalesce(x.new_episodes, 1) = 1 AND u.id IN (SELECT user_id FROM tmdb_follows WHERE kind = 'show' AND tmdb_id = ?) ORDER BY u.id`, tmdbID)
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
			_, err := tx.Exec("DELETE FROM tmdb_follows WHERE kind = 'show' AND tmdb_id = ?", tmdbID)
			return err
		})
		return 0, nil
	}
	now := store.FormatTime(s.now())
	text := name + " is in the Library"
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, uid := range all {
			if _, err := tx.Exec("INSERT INTO follows (user_id, title_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", uid, titleID, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("DELETE FROM tmdb_follows WHERE kind = 'show' AND tmdb_id = ?", tmdbID); err != nil {
			return err
		}
		for _, p := range tell {
			if _, err := tx.Exec("INSERT INTO messages (created_at, user_id, kind, text, title_id) VALUES (?, ?, 'new_episodes', ?, ?)", now, p.ID, text+".", titleID); err != nil {
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
