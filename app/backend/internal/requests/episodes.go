package requests

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// EpisodeBatch is how long new episodes of one show are gathered before
// the people who asked for it hear, so a pack is one notice, not ten.
const EpisodeBatch = time.Hour

// WatchEpisodes notes every episode file placed in the Library, for
// SendEpisodeNotices. Upgrades of an episode already noted are not new.
func (s *Service) WatchEpisodes() {
	s.Store.OnEvent(func(e domain.Event) {
		if (e.Kind != "import_done" && e.Kind != "import_partly_done") || e.TitleID == nil {
			return
		}
		s.sending.Add(1)
		go func() {
			defer s.sending.Done()
			jobs.Guard("new_episodes", func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if err := s.noteEpisodes(ctx, e); err != nil {
					log.Printf("asks: new episodes: %v", err)
				}
			})
		}()
	})
}

func (s *Service) noteEpisodes(ctx context.Context, e domain.Event) error {
	ids := alerts.SlotIDsOf(ctx, s.Store.Read(), e)
	if len(ids) == 0 {
		return nil
	}
	now := database.FormatTime(s.now())
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.Exec(`INSERT INTO new_episode_notices (title_id, slot_id, arrived_at)
				SELECT i.title_id, s.id, ? FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
				WHERE s.id = ? AND i.kind = 'episode' AND t.kind = 'show'
				ON CONFLICT (slot_id) DO NOTHING`, now, id); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO new_album_notices (title_id, part_id, arrived_at)
				SELECT i.title_id, i.part_id, ? FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
				WHERE s.id = ? AND i.kind = 'track' AND t.kind = 'artist' AND i.part_id IS NOT NULL
				ON CONFLICT (part_id) DO NOTHING`, now, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// SendEpisodeNotices tells the people who asked for a show about its new
// episodes, once the show's first unsent episode is EpisodeBatch old: one
// notice per show ("3 new episodes of Slow Horses"), in the app and through
// the Alerts for the new_episodes moment. People who turned new-episode
// notices off, and asks turned down, are skipped. It returns how many shows
// it told about.
func (s *Service) SendEpisodeNotices(ctx context.Context) (int, error) {
	due := database.FormatTime(s.now().Add(-EpisodeBatch))
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT n.title_id, t.name, count(*) FROM new_episode_notices n JOIN titles t ON t.id = n.title_id
		WHERE n.sent_at IS NULL GROUP BY n.title_id HAVING min(n.arrived_at) <= ? ORDER BY n.title_id`, due)
	if err != nil {
		return 0, err
	}
	type show struct {
		id   int64
		name string
		n    int
	}
	var shows []show
	for rows.Next() {
		var sh show
		if err := rows.Scan(&sh.id, &sh.name, &sh.n); err != nil {
			rows.Close()
			return 0, err
		}
		shows = append(shows, sh)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	told := 0
	for _, sh := range shows {
		people, askID, err := s.showPeople(ctx, sh.id)
		if err != nil {
			return told, err
		}
		now := database.FormatTime(s.now())
		text := EpisodeWords(sh.n, sh.name)
		err = s.Store.Write(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec("UPDATE new_episode_notices SET sent_at = ? WHERE title_id = ? AND sent_at IS NULL", now, sh.id); err != nil {
				return err
			}
			for _, p := range people {
				if _, err := tx.Exec("INSERT INTO messages (created_at, user_id, kind, text, ask_id, title_id) VALUES (?, ?, 'new_episodes', ?, ?, ?)",
					now, p.ID, text+".", askID[p.ID], sh.id); err != nil {
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
		m := alerts.Message{Moment: alerts.MomentNewEpisodes, Subject: text, Text: text + " are in the Library and ready to watch."}
		if sh.n == 1 {
			m.Text = text + " is in the Library and ready to watch."
		}
		for _, p := range people {
			m.People = append(m.People, alerts.Person{Name: p.Name, Email: p.Email, DiscordID: p.DiscordID})
		}
		s.send(ctx, m)
	}
	return told, nil
}

// EpisodeWords says "A new episode of X" or "3 new episodes of X".
func EpisodeWords(n int, show string) string {
	if n == 1 {
		return "A new episode of " + show
	}
	return strconv.Itoa(n) + " new episodes of " + show
}

// showPeople are the people on open or done asks for a show, and the
// people who follow it, who want new-episode notices, with their ask (nil
// for a follower who did not ask).
func (s *Service) showPeople(ctx context.Context, titleID int64) ([]person, map[int64]*int64, error) {
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, ''), min(w.ask_id)
		FROM (SELECT p.user_id, a.id AS ask_id FROM asks a JOIN ask_people p ON p.ask_id = a.id
				WHERE a.kind = 'show' AND a.title_id = ? AND a.state IN ('approved', 'available')
			UNION ALL SELECT f.user_id, NULL FROM follows f WHERE f.title_id = ?) w
		JOIN users u ON u.id = w.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		WHERE u.state != 'disabled' AND coalesce(x.new_episodes, 1) = 1
		GROUP BY u.id ORDER BY u.id`, titleID, titleID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []person
	asks := map[int64]*int64{}
	for rows.Next() {
		var p person
		var ask *int64
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.DiscordID, &ask); err != nil {
			return nil, nil, err
		}
		p.Name = strings.TrimSpace(p.Name)
		out = append(out, p)
		asks[p.ID] = ask
	}
	return out, asks, rows.Err()
}
