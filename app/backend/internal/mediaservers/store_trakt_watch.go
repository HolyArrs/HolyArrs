package mediaservers

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// traktReadKey keeps when the Trakt watch history was last read whole.
const traktReadKey = watchReadKey + "trakt"

// replaceTraktWatchState swaps the Trakt rows for a whole pass of the
// person who signed in (a sign-in by someone else drops the old person's
// rows), unless the owner turned watch history off while it was read.
func replaceTraktWatchState(ctx context.Context, s *database.Store, userID int64, name string, rows []watchRow, at time.Time) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		if !WatchHistoryOn(ctx, tx) {
			return nil
		}
		if _, err := tx.Exec("DELETE FROM trakt_watch_state"); err != nil {
			return err
		}
		st, err := tx.Prepare(`INSERT INTO trakt_watch_state(user_id, account_name, title_id, kind, season, episode, plays, last_watched_at, read_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		readAt := database.FormatTime(at)
		if err := database.PutSetting(tx, watchGroup, traktReadKey, readAt, domain.System, at); err != nil {
			return err
		}
		for _, r := range rows {
			var last any
			if r.last != nil {
				last = database.FormatTime(*r.last)
			}
			if _, err := st.Exec(userID, name, r.title, r.kind, r.season, r.episode, r.plays, last, readAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// clearTraktWatchState deletes the Trakt rows: nobody is signed in now.
func clearTraktWatchState(ctx context.Context, s *database.Store) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("DELETE FROM trakt_watch_state")
		if n, _ := res.RowsAffected(); err != nil || n == 0 {
			return err
		}
		return database.PutSetting(tx, watchGroup, traktReadKey, "", domain.System, time.Now().UTC())
	})
}

// traktWatchedRows sums the Trakt rows per person, as WatchedPerson lines
// (Server "Trakt"). The person is linked by the sign-in; the caller fills
// the person's name.
func traktWatchedRows(ctx context.Context, q database.Lister) ([]WatchedPerson, error) {
	rows, err := q.QueryContext(ctx, `SELECT user_id, max(account_name),
		sum(kind = 'movie'), count(DISTINCT CASE WHEN kind = 'episode' THEN title_id END), sum(kind = 'episode'), sum(plays), max(last_watched_at)
		FROM trakt_watch_state GROUP BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WatchedPerson
	for rows.Next() {
		p := WatchedPerson{Server: "Trakt", ServerKind: "trakt", Linked: true}
		var id int64
		var last sql.NullString
		if err := rows.Scan(&id, &p.Name, &p.Movies, &p.Shows, &p.Episodes, &p.Plays, &last); err != nil {
			return nil, err
		}
		p.UserID, p.account = &id, strconv.FormatInt(id, 10)
		if last.Valid {
			p.LastWatchedAt = &last.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// eachTraktWatchedBy calls fn for every movie (season and episode 0) and
// episode the person watched on Trakt.
func eachTraktWatchedBy(ctx context.Context, q database.Lister, userID int64, fn func(title int64, season, episode int)) error {
	rows, err := q.QueryContext(ctx, "SELECT title_id, season, episode FROM trakt_watch_state WHERE user_id = ?", userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var title int64
		var season, episode int
		if err := rows.Scan(&title, &season, &episode); err != nil {
			return err
		}
		fn(title, season, episode)
	}
	return rows.Err()
}
