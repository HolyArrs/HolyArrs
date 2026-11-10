package alerts

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// MomentOf says which Library moment an Event is, or "" when Alerts do not
// speak for it. An import that placed nothing is not an import moment (the
// Slot then needs a decision, which is its own moment).
func MomentOf(e domain.Event) string {
	switch e.Kind {
	case "download_sent":
		return MomentGrab
	case "import_done", "import_partly_done":
		return MomentImport
	case domain.EventRenamed:
		return MomentRename
	case domain.EventTitleAdded:
		return MomentAdd
	case domain.EventTitleRemoved, domain.EventEpisodesRemoved:
		return MomentDelete
	case domain.EventHealthBroken, domain.EventHealthRecovered:
		return MomentHealth
	case domain.EventNeedsDecision:
		return MomentDecision
	case "subtitle_saved", "subtitle_upgraded", "subtitle_failed", "subtitle_site_paused":
		return MomentSubtitle
	}
	return ""
}

var momentWords = words["en"]

// MessageFor builds the message for a Library Event (ok false when Alerts
// do not speak for it).
func (s *Sender) MessageFor(ctx context.Context, e domain.Event) (Message, bool) {
	moment := MomentOf(e)
	if moment == "" {
		return Message{}, false
	}
	m := Message{Moment: moment, Event: &e, Text: e.Summary, lang: s.lang(ctx)}
	db := s.Store.Read()
	if e.TitleID != nil {
		var mv Movie
		var kind string
		var year *int
		var tmdb, imdb, tvdb, maze, mb *string
		err := db.QueryRowContext(ctx, `SELECT id, kind, name, year, json_extract(source_ids, '$.tmdb'), json_extract(source_ids, '$.imdb'),
			json_extract(source_ids, '$.tvdb'), json_extract(source_ids, '$.tvmaze'), json_extract(source_ids, '$.musicbrainz') FROM titles WHERE id = ?`,
			*e.TitleID).Scan(&mv.ID, &kind, &mv.Name, &year, &tmdb, &imdb, &tvdb, &maze, &mb)
		str := func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		}
		switch {
		case err != nil:
		case kind == "artist":
			m.Artist = &Artist{ID: mv.ID, Name: mv.Name, MusicBrainz: str(mb)}
			if e.Kind != domain.EventEpisodesRemoved && e.Kind != domain.EventTitleRemoved {
				m.Tracks = tracksOf(ctx, db, slotIDsOf(ctx, db, e))
			}
		case kind == "show":
			m.Show = &Show{ID: mv.ID, Name: mv.Name, Year: year, TMDB: str(tmdb), IMDb: str(imdb), TVDB: str(tvdb), TVMaze: str(maze)}
			if e.Kind == domain.EventEpisodesRemoved {
				m.Episodes = episodesFromLabels(e)
			} else if e.Kind != domain.EventTitleRemoved {
				m.Episodes = episodesOf(ctx, db, slotIDsOf(ctx, db, e))
			}
		default:
			mv.Year, mv.TMDB, mv.IMDb = year, str(tmdb), str(imdb)
			m.Movie = &mv
		}
	}
	name := ""
	switch {
	case m.Movie != nil:
		name = m.Movie.Name
		if m.Movie.Year != nil {
			name += fmt.Sprintf(" (%d)", *m.Movie.Year)
		}
	case m.Artist != nil:
		// An artist is named with its album: "Lab Lanterns – Small Hours (2025)".
		name = m.Artist.Name
		if w := trackWordsIn(m.lang, m.Tracks); w != "" {
			name += " – " + w
		} else if album, _ := e.Data["album"].(string); album != "" {
			name += " – " + album
		}
		if len(m.Tracks) > 1 || len(m.Tracks) == 1 && m.Tracks[0].Name != "" {
			m.Text += "\n" + trackLines(m.Tracks)
		}
	case m.Show != nil:
		// A show is named with its episodes: "Copper Valley S01E02".
		name = strings.TrimSpace(m.Show.Name + " " + episodeWordsIn(m.lang, m.Episodes))
		if len(m.Episodes) > 1 || len(m.Episodes) == 1 && m.Episodes[0].Name != "" {
			m.Text += "\n" + episodeLines(m.Episodes)
		}
	}
	switch {
	case moment == MomentHealth && e.Kind == domain.EventHealthRecovered:
		m.Subject = say(m.lang, "health_fixed")
	case moment == MomentHealth:
		m.Subject = say(m.lang, "health_look")
	case name != "":
		m.Subject = say(m.lang, moment) + ": " + name
	default:
		m.Subject = say(m.lang, moment)
	}
	var extra []string
	for _, k := range []string{"reason", "release", "downloader", "save_path", "from", "message"} {
		if v, ok := e.Data[k].(string); ok && v != "" && !strings.Contains(m.Text, v) {
			v = payloadData(v).(string) // a full path of this machine: its last name only
			extra = append(extra, k+": "+v)
		}
	}
	if len(extra) > 0 {
		m.Text += "\n" + strings.Join(extra, "\n")
	}
	return m, true
}

// Watch sends every stored Library Event through the Alerts that speak for
// its moment, each on its own goroutine (Wait waits for them); ending base
// stops the sends and their new tries.
func (s *Sender) Watch(base context.Context) {
	s.Store.OnEvent(func(e domain.Event) {
		if MomentOf(e) == "" {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			jobs.Guard("alerts", func() {
				ctx, cancel := context.WithTimeout(base, 10*time.Minute)
				defer cancel()
				for _, m := range s.MessagesFor(ctx, e) {
					if err := s.Send(ctx, m); err != nil {
						log.Printf("alerts: %s: %s", m.Moment, redact(err.Error()))
					}
				}
			})
		}()
	})
}

// MessagesFor builds the messages for a Library Event: one, or with the
// pack_alerts setting each, one per episode of a pack import, at most
// each_limit of them; the rest go in one summary message.
func (s *Sender) MessagesFor(ctx context.Context, e domain.Event) []Message {
	m, ok := s.MessageFor(ctx, e)
	if !ok {
		return nil
	}
	if m.Moment == MomentImport && m.Artist != nil && len(m.Tracks) >= 2 {
		return s.trackMessages(ctx, m)
	}
	if m.Moment != MomentImport || m.Show == nil || len(m.Episodes) < 2 {
		return []Message{m}
	}
	st := ReadSettings(ctx, s.Store.Read())
	if st.PackAlerts != PackEach {
		return []Message{m}
	}
	eps := m.Episodes
	rest := []Episode(nil)
	if len(eps) > st.EachLimit {
		eps, rest = eps[:st.EachLimit], eps[st.EachLimit:]
	}
	out := make([]Message, 0, len(eps)+1)
	for _, ep := range eps {
		one := m
		one.Episodes = []Episode{ep}
		one.Subject = say(m.lang, m.Moment) + ": " + m.Show.Name + " " + ep.Label()
		one.Text = say(m.lang, "in_library", "name", strings.TrimSpace(ep.Label()+" "+ep.Name))
		out = append(out, one)
	}
	if len(rest) > 0 {
		sum := m
		sum.Episodes = rest
		sum.Subject = say(m.lang, m.Moment) + ": " + m.Show.Name + " " + episodeWordsIn(m.lang, rest)
		sum.Text = say(m.lang, "more_episodes", "n", strconv.Itoa(len(rest))) + "\n" + episodeLines(rest)
		out = append(out, sum)
	}
	return out
}

// trackMessages is one message for an album import, or with pack_alerts
// each one per track, at most each_limit of them, the rest in one summary.
func (s *Sender) trackMessages(ctx context.Context, m Message) []Message {
	st := ReadSettings(ctx, s.Store.Read())
	if st.PackAlerts != PackEach {
		return []Message{m}
	}
	ts, rest := m.Tracks, []Track(nil)
	if len(ts) > st.EachLimit {
		ts, rest = ts[:st.EachLimit], ts[st.EachLimit:]
	}
	out := make([]Message, 0, len(ts)+1)
	for _, t := range ts {
		one := m
		one.Tracks = []Track{t}
		one.Subject = say(m.lang, m.Moment) + ": " + m.Artist.Name + " – " + trackWordsIn(m.lang, one.Tracks)
		one.Text = say(m.lang, "in_library", "name", strings.TrimSpace(t.albumName()+" "+t.Label()+" "+t.Name))
		out = append(out, one)
	}
	if len(rest) > 0 {
		sum := m
		sum.Tracks = rest
		sum.Subject = say(m.lang, m.Moment) + ": " + m.Artist.Name + " – " + trackWordsIn(m.lang, rest)
		sum.Text = say(m.lang, "more_tracks", "n", strconv.Itoa(len(rest))) + "\n" + trackLines(rest)
		out = append(out, sum)
	}
	return out
}

// Wait waits for the sends Watch started.
func (s *Sender) Wait() { s.wg.Wait() }
