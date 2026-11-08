package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ErrNotFollowable says the title is not a movie, a show or an artist in
// the Library.
var ErrNotFollowable = errors.New("only a movie, a show or an artist in the Library can be followed")

// Followed is one title a person follows.
type Followed struct {
	TitleID    int64  `json:"title_id"`
	Kind       string `json:"kind"` // movie, show, artist or album
	Name       string `json:"name"`
	Year       *int   `json:"year,omitempty"`
	FollowedAt string `json:"followed_at"`
	// A follow of something not in the Library: its TMDB id (movie, show) or
	// MusicBrainz id and artist (album). Name is empty for TMDB ones (the app
	// looks it up). Unfollow is the address that stops it (DELETE).
	TMDBID   string `json:"tmdb_id,omitempty"`
	MBID     string `json:"mbid,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Unfollow string `json:"unfollow"`
}

// Follow puts a title's dates on p's calendar and lets p hear about a
// show's new episodes and seasons or an artist's new albums. It asks for
// nothing and changes nothing in the Library (Kept up to date is separate).
// A Kid may follow only titles within their limit.
func (s *Service) Follow(ctx context.Context, p Person, titleID int64) error {
	return s.follow(ctx, p, titleID, false)
}

// FollowTitle is Follow from a title's bell. A movie followed before its
// file is in is also marked as still to tell, so the person hears once
// when the file arrives (TellMovieFollows). Not when it already has a file
// or when the person asked for it (the ask tells them).
func (s *Service) FollowTitle(ctx context.Context, p Person, titleID int64) error {
	return s.follow(ctx, p, titleID, true)
}

func (s *Service) follow(ctx context.Context, p Person, titleID int64, tellOnFile bool) error {
	db := s.Store.Read()
	var kind string
	err := db.QueryRowContext(ctx, followSQL, titleID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && kind != "movie" && kind != "show" && kind != "artist") {
		return ErrNotFollowable
	}
	if err != nil {
		return err
	}
	if p.MaxRating != "" {
		if err := libraryOverLimit(ctx, db, p, kind, titleID); err != nil {
			return err
		}
	}
	now := database.FormatTime(s.now())
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(followSQL2, p.UserID, titleID, now)
		if err != nil || !tellOnFile || kind != "movie" {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		_, err = tx.Exec(followSQL3,
			p.UserID, now, titleID, p.UserID)
		return err
	})
}

// FollowAsk records whether a person who asked follows the asked title,
// and, when the ask already has its Library title, starts (or ends) the
// follow now. Approving an ask starts the follows of its askers who chose
// to follow (followAskers).
func (s *Service) FollowAsk(ctx context.Context, p Person, askID int64, follow bool) error {
	var tid sql.NullInt64
	if err := s.Store.Read().QueryRowContext(ctx, followAskSQL, askID).Scan(&tid); err != nil {
		return err
	}
	on := 0
	if follow {
		on = 1
	}
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(followAskSQL2, on, askID, p.UserID)
		return err
	}); err != nil {
		return err
	}
	if !tid.Valid || !follow {
		return nil
	}
	err := s.Follow(ctx, p, tid.Int64)
	if errors.Is(err, ErrNotFollowable) || errors.Is(err, ErrAboveLimit) {
		return nil
	}
	return err
}

// followAskers starts the follows of an ask's askers who chose to follow,
// once the ask has its Library title.
func (s *Service) followAskers(ctx context.Context, askID, titleID int64) error {
	now := database.FormatTime(s.now())
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(followAskersSQL, titleID, now, askID)
		return err
	})
}

// FollowAdded makes a person follow a title they added by hand or brought
// in from their own folders. Titles above a Kid's limit are skipped.
func (s *Service) FollowAdded(ctx context.Context, p Person, titleIDs ...int64) error {
	for _, id := range titleIDs {
		err := s.Follow(ctx, p, id)
		if err != nil && !errors.Is(err, ErrNotFollowable) && !errors.Is(err, ErrAboveLimit) {
			return err
		}
	}
	return nil
}

// Unfollow stops a person's follow of a title (no follow: nothing to do).
// For a movie followed before it was in the Library, the wait for its file
// ends too, so the person is not told about it later.
func (s *Service) Unfollow(ctx context.Context, userID, titleID int64) error {
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(unfollowSQL, userID, titleID); err != nil {
			return err
		}
		_, err := tx.Exec(unfollowSQL2, userID, titleID)
		return err
	})
}

// IsFollowing says whether a person follows a title.
func (s *Service) IsFollowing(ctx context.Context, userID, titleID int64) (bool, error) {
	var n int
	err := s.Store.Read().QueryRowContext(ctx, isFollowingSQL, userID, titleID).Scan(&n)
	return n > 0, err
}

// Following lists what a person follows, by name.
func (s *Service) Following(ctx context.Context, userID int64) ([]Followed, error) {
	rows, err := s.Store.Read().QueryContext(ctx, followingSQL, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Followed{}
	for rows.Next() {
		var f Followed
		var year sql.NullInt64
		if err := rows.Scan(&f.TitleID, &f.Kind, &f.Name, &year, &f.FollowedAt); err != nil {
			return nil, err
		}
		if year.Valid {
			y := int(year.Int64)
			f.Year = &y
		}
		f.Unfollow = fmt.Sprintf("/titles/%d/follow", f.TitleID)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	// Follows of titles not in the Library yet: shows and movies by TMDB id
	// (skipping any whose title is now followed in the Library), then albums.
	more, err := s.Store.Read().QueryContext(ctx, followingSQL2, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer more.Close()
	for more.Next() {
		var f Followed
		var id string
		if err := more.Scan(&f.Kind, &id, &f.Name, &f.Artist, &f.FollowedAt); err != nil {
			return nil, err
		}
		if f.Kind == "album" {
			f.MBID = id
			f.Unfollow = "/metadata/album/" + url.PathEscape(id) + "/follow"
		} else {
			f.TMDBID = id
			f.Unfollow = "/metadata/" + f.Kind + "/" + url.PathEscape(id) + "/follow"
		}
		out = append(out, f)
	}
	return out, more.Err()
}

// SendAlbumNotices tells the people who follow an artist about its new
// albums, once the artist's first unsent album is EpisodeBatch old: one
// notice per artist, in the app and through the Alerts for the new_albums
// moment. It returns how many artists it told about.
func (s *Service) SendAlbumNotices(ctx context.Context) (int, error) {
	due := database.FormatTime(s.now().Add(-EpisodeBatch))
	rows, err := s.Store.Read().QueryContext(ctx, sendAlbumNoticesSQL, due)
	if err != nil {
		return 0, err
	}
	type artist struct {
		id   int64
		name string
		n    int
	}
	var as []artist
	for rows.Next() {
		var a artist
		if err := rows.Scan(&a.id, &a.name, &a.n); err != nil {
			rows.Close()
			return 0, err
		}
		as = append(as, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	told := 0
	for _, a := range as {
		people, err := s.followers(ctx, a.id)
		if err != nil {
			return told, err
		}
		now := database.FormatTime(s.now())
		text := AlbumWords(a.n, a.name)
		err = s.Store.Write(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec(sendAlbumNoticesSQL2, now, a.id); err != nil {
				return err
			}
			for _, p := range people {
				if _, err := tx.Exec(sendAlbumNoticesSQL3,
					now, p.ID, text+".", a.id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return told, err
		}
		if len(people) == 0 {
			continue
		}
		told++
		m := alerts.Message{Moment: alerts.MomentNewAlbums, Subject: text, Text: text + " is in the Library and ready to play."}
		if a.n > 1 {
			m.Text = text + " are in the Library and ready to play."
		}
		for _, p := range people {
			m.People = append(m.People, alerts.Person{Name: p.Name, Email: p.Email, DiscordID: p.DiscordID})
		}
		s.send(ctx, m)
	}
	return told, nil
}

// AlbumWords says "A new album from X" or "2 new albums from X".
func AlbumWords(n int, artist string) string {
	if n == 1 {
		return "A new album from " + artist
	}
	return strconv.Itoa(n) + " new albums from " + artist
}

// followers are the people who follow a title and want new-part notices.
func (s *Service) followers(ctx context.Context, titleID int64) ([]person, error) {
	rows, err := s.Store.Read().QueryContext(ctx, followersSQL, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []person
	for rows.Next() {
		var p person
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.DiscordID); err != nil {
			return nil, err
		}
		p.Name = strings.TrimSpace(p.Name)
		out = append(out, p)
	}
	return out, rows.Err()
}
