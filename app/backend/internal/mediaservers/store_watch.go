package mediaservers

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// watchGroup and watchKey hold the owner's "Read watch history" switch
// ("on"; anything else is off).
const watchGroup, watchKey = "links", "watch_history"

// watchReadKey starts the settings that keep when each media server's watch
// history was last read whole (the server's id follows it; "" for not since
// it was turned on). A pass that matches nothing in the Library leaves no
// rows, so the rows alone cannot say a read happened.
const watchReadKey = "watch_history_read:"

// WatchHistoryOn says whether the owner turned watch history on.
func WatchHistoryOn(ctx context.Context, q database.Querier) bool {
	return database.SettingOr(ctx, q, watchGroup, watchKey, "") == "on"
}

// SetWatchHistory turns watch history on or off; off deletes every row
// in the same step.
func SetWatchHistory(ctx context.Context, s *database.Store, on bool, by domain.Actor) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		v := "off"
		if on {
			v = "on"
		}
		if err := database.PutSetting(tx, watchGroup, watchKey, v, by, time.Now().UTC()); err != nil {
			return err
		}
		if on {
			return nil
		}
		if _, err := tx.Exec("DELETE FROM watch_state"); err != nil {
			return err
		}
		var read []string
		if err := database.EachSetting(ctx, tx, watchGroup, func(k, _ string) {
			if strings.HasPrefix(k, watchReadKey) {
				read = append(read, k)
			}
		}); err != nil {
			return err
		}
		for _, k := range read {
			if err := database.PutSetting(tx, watchGroup, k, "", by, time.Now().UTC()); err != nil {
				return err
			}
		}
		return nil
	})
}

// watchRow is one thing one account watched.
type watchRow struct {
	account, name   string
	title           int64
	kind            string
	season, episode int
	plays           int
	last            *time.Time
}

// replaceWatchState swaps one server's rows for a whole pass, unless the
// owner turned watch history off while it was read.
func replaceWatchState(ctx context.Context, s *database.Store, serverID int64, rows []watchRow, at time.Time) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		if !WatchHistoryOn(ctx, tx) {
			return nil
		}
		if _, err := tx.Exec("DELETE FROM watch_state WHERE media_server_id = ?", serverID); err != nil {
			return err
		}
		st, err := tx.Prepare(`INSERT INTO watch_state(media_server_id, account_key, account_name, title_id, kind, season, episode, plays, last_watched_at, read_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		readAt := database.FormatTime(at)
		if err := database.PutSetting(tx, watchGroup, watchReadKey+strconv.FormatInt(serverID, 10), readAt, domain.System, at); err != nil {
			return err
		}
		for _, r := range rows {
			var last any
			if r.last != nil {
				last = database.FormatTime(*r.last)
			}
			if _, err := st.Exec(serverID, r.account, r.name, r.title, r.kind, r.season, r.episode, r.plays, last, readAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// WatchedPerson is what one account on one media server watched.
type WatchedPerson struct {
	Server     string `json:"server"`
	ServerKind string `json:"server_kind"`
	// Name is the account's name on the media server.
	Name string `json:"name"`
	// Linked says the account belongs to a person here (UserID, UserName).
	Linked        bool    `json:"linked"`
	UserID        *int64  `json:"user_id,omitempty"`
	UserName      string  `json:"user_name,omitempty"`
	Movies        int     `json:"movies"`
	Shows         int     `json:"shows"`
	Episodes      int     `json:"episodes"`
	Plays         int     `json:"plays"`
	LastWatchedAt *string `json:"last_watched_at,omitempty"`

	account string // the account's id on the server
}

// WatchedSummary is who watched what, for people who manage the Library.
type WatchedSummary struct {
	Enabled bool            `json:"enabled"`
	ReadAt  *string         `json:"read_at,omitempty"`
	People  []WatchedPerson `json:"people"`
}

// watchReader reads rows and single values (a pool or a transaction).
type watchReader interface {
	database.Lister
	database.Querier
}

// watchedRows sums watch_state per account, most plays first.
func watchedRows(ctx context.Context, q watchReader) (WatchedSummary, error) {
	out := WatchedSummary{Enabled: WatchHistoryOn(ctx, q), People: []WatchedPerson{}}
	var readAt sql.NullString
	if err := q.QueryRowContext(ctx, "SELECT max(read_at) FROM watch_state").Scan(&readAt); err != nil {
		return out, err
	}
	// The newest whole read of a server still here, rows or not.
	ms, err := listServers(ctx, q)
	if err != nil {
		return out, err
	}
	here := map[string]bool{}
	for _, m := range ms {
		here[watchReadKey+strconv.FormatInt(m.ID, 10)] = true
	}
	if err := database.EachSetting(ctx, q, watchGroup, func(k, v string) {
		if here[k] && v > readAt.String {
			readAt = sql.NullString{String: v, Valid: true}
		}
	}); err != nil {
		return out, err
	}
	if readAt.Valid {
		out.ReadAt = &readAt.String
	}
	rows, err := q.QueryContext(ctx, `SELECT ms.name, ms.kind, ws.account_key, max(ws.account_name),
		sum(ws.kind = 'movie'), count(DISTINCT CASE WHEN ws.kind = 'episode' THEN ws.title_id END), sum(ws.kind = 'episode'), sum(ws.plays), max(ws.last_watched_at)
		FROM watch_state ws JOIN media_servers ms ON ms.id = ws.media_server_id
		GROUP BY ws.media_server_id, ws.account_key ORDER BY 8 DESC, 4`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var p WatchedPerson
		var last sql.NullString
		if err := rows.Scan(&p.Server, &p.ServerKind, &p.account, &p.Name, &p.Movies, &p.Shows, &p.Episodes, &p.Plays, &last); err != nil {
			return out, err
		}
		if last.Valid {
			p.LastWatchedAt = &last.String
		}
		out.People = append(out.People, p)
	}
	return out, rows.Err()
}
