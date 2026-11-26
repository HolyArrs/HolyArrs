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
	case "download_finished":
		return MomentDownloaded
	case "download_failed":
		// A Release that keeps stalling speaks once more on its second stall,
		// then stays quiet while it waits; History still lists each one.
		if stallCount(e.Data) > 2 {
			return ""
		}
		return MomentDownloadFailed
	case "import_done", "import_partly_done":
		return MomentImport
	case domain.EventRenamed:
		return MomentRename
	case domain.EventTitleAdded:
		return MomentAdd
	case domain.EventArtworkChanged:
		return MomentArtwork
	case domain.EventTitleRemoved, domain.EventEpisodesRemoved, domain.EventMovieFileRemoved:
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
	s.titleInto(ctx, e, &m)
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
	if moment == MomentImport && e.Data["upgraded"] == true {
		m.head = "upgraded"
		if lines := upgradeLines(e.Data); lines != "" {
			m.Text += "\n" + lines
		}
	}
	switch {
	case moment == MomentHealth && e.Kind == domain.EventHealthRecovered:
		m.Subject = say(m.lang, "health_fixed")
	case moment == MomentHealth:
		m.Subject = say(m.lang, "health_look")
	case name != "":
		m.Subject = m.headWord() + ": " + name
	default:
		m.Subject = m.headWord()
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

// titleInto fills m's movie, show and episodes, or artist and tracks, from
// the title e is about.
func (s *Sender) titleInto(ctx context.Context, e domain.Event, m *Message) {
	db := s.Store.Read()
	if e.TitleID != nil {
		var mv Movie
		var kind string
		var year *int
		var tmdb, imdb, tvdb, maze, mb *string
		var poster string
		err := db.QueryRowContext(ctx, `SELECT id, kind, name, year, json_extract(source_ids, '$.tmdb'), json_extract(source_ids, '$.imdb'),
			json_extract(source_ids, '$.tvdb'), json_extract(source_ids, '$.tvmaze'), json_extract(source_ids, '$.musicbrainz'), poster_url FROM titles WHERE id = ?`,
			*e.TitleID).Scan(&mv.ID, &kind, &mv.Name, &year, &tmdb, &imdb, &tvdb, &maze, &mb, &poster)
		if err == nil && kind != "artist" {
			m.Poster = poster
		}
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
			m.Poster = s.musicPicture(m.Artist.MusicBrainz, m.Tracks)
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
				ms := s.MessagesFor(ctx, e)
				if s.Tap != nil && len(ms) > 0 {
					// Plugins get one message per Event, whatever the
					// owner's pack setting split it into.
					if whole, ok := s.MessageFor(ctx, e); ok && len(ms) > 1 {
						s.Tap(whole)
					} else {
						s.Tap(ms[0])
					}
				}
				for _, m := range ms {
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
		one.Subject = m.headWord() + ": " + m.Show.Name + " " + ep.Label()
		one.Text = say(m.lang, "in_library", "name", strings.TrimSpace(ep.Label()+" "+ep.Name))
		out = append(out, one)
	}
	if len(rest) > 0 {
		sum := m
		sum.Episodes = rest
		sum.Subject = m.headWord() + ": " + m.Show.Name + " " + episodeWordsIn(m.lang, rest)
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
		one.Subject = m.headWord() + ": " + m.Artist.Name + " – " + trackWordsIn(m.lang, one.Tracks)
		one.Text = say(m.lang, "in_library", "name", strings.TrimSpace(t.albumName()+" "+t.Label()+" "+t.Name))
		out = append(out, one)
	}
	if len(rest) > 0 {
		sum := m
		sum.Tracks = rest
		sum.Subject = m.headWord() + ": " + m.Artist.Name + " – " + trackWordsIn(m.lang, rest)
		sum.Text = say(m.lang, "more_tracks", "n", strconv.Itoa(len(rest))) + "\n" + trackLines(rest)
		out = append(out, sum)
	}
	return out
}

// Wait waits for the sends Watch started.
func (s *Sender) Wait() { s.wg.Wait() }

// stallCount reads an Event's stall_count, a number in memory or from JSON.
func stallCount(data map[string]any) int {
	switch n := data["stall_count"].(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

// headWord is the word a Library moment's subject starts with.
func (m Message) headWord() string {
	if m.head != "" {
		return say(m.lang, m.head)
	}
	return say(m.lang, m.Moment)
}

// upgradeLines shows what an upgrade replaced: "quality: 720p → 1080p",
// the release names and sizes; a side that is not known is "?".
func upgradeLines(data map[string]any) string {
	var out []string
	for _, k := range []string{"quality", "release", "size"} {
		was, now := factText(k, data["old_"+k]), factText(k, data["new_"+k])
		if was == "" && now == "" {
			continue
		}
		if was == "" {
			was = "?"
		}
		if now == "" {
			now = "?"
		}
		out = append(out, k+": "+was+" → "+now)
	}
	return strings.Join(out, "\n")
}

// factText is an upgrade fact as text: sizes in bytes (a number in memory,
// a float once read back from JSON) become "4.2 GB".
func factText(key string, v any) string {
	if key != "size" {
		s, _ := v.(string)
		return s
	}
	var n float64
	switch x := v.(type) {
	case int64:
		n = float64(x)
	case int:
		n = float64(x)
	case float64:
		n = x
	default:
		return ""
	}
	if n <= 0 {
		return ""
	}
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", n/(1<<30))
	}
	return fmt.Sprintf("%.0f MB", n/(1<<20))
}

// musicPicture is the metadata server's public picture for a music
// message: the album cover when the tracks are all from one album, else
// the artist photo; "" with no picture address or no MusicBrainz id.
func (s *Sender) musicPicture(artist string, tracks []Track) string {
	base := strings.TrimRight(s.Pictures, "/")
	if base == "" {
		return ""
	}
	album := ""
	for i, t := range tracks {
		if i > 0 && t.albumMBID != album {
			album = ""
			break
		}
		album = t.albumMBID
	}
	if validMBID(album) {
		return base + "/album/" + album + "/500.webp"
	}
	if validMBID(artist) {
		return base + "/artist/" + strings.ToLower(artist) + "/500.webp"
	}
	return ""
}

// validMBID accepts a MusicBrainz id (8-4-4-4-12 hex), so nothing else is
// put in an address.
func validMBID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range strings.ToLower(id) {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
	}
	return true
}
