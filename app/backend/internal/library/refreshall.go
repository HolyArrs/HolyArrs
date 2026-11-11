package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// KindRefreshTitles is the 24 h details refresh, or every movie by hand.
const KindRefreshTitles = "refresh_titles"

// Refresh rule numbers (Radarr's movie refresh, backend/jobs.md task 7).
const (
	refreshStale   = 180 * 24 * time.Hour // always refresh after this
	refreshFresh   = 12 * time.Hour       // never refresh before this
	physicalWindow = 30 * 24 * time.Hour  // refresh while the physical release is this recent
	changedListAge = 14 * 24 * time.Hour  // the changed list is used when the last run is newer
)

// ScoreFillPerRun is how many movies, and how many shows, with no score yet
// a scheduled refresh adds, so a large Library fills over a few runs.
const ScoreFillPerRun = 300

// NeedsScore is true for a title not looked up since scores were kept,
// not dropped by the source and not fetched within fresh.
func NeedsScore(t mediafiles.Title, now time.Time, fresh time.Duration) bool {
	return t.Score == nil && t.DroppedAt == nil && (t.RefreshedAt == nil || now.Sub(*t.RefreshedAt) >= fresh)
}

// ShouldRefresh applies the refresh rules, in order, to a Title that is not
// on the changed list: over 180 days since the last fetch, yes; under 12
// hours, no; announced or in cinemas, yes; released with the physical date
// in the last 30 days, yes; otherwise no. why says which rule decided.
func ShouldRefresh(t mediafiles.Title, now time.Time) (yes bool, why string) {
	if t.RefreshedAt == nil {
		return true, "never fetched"
	}
	age := now.Sub(*t.RefreshedAt)
	switch {
	case age > refreshStale:
		return true, "last fetch over 180 days ago"
	case age < refreshFresh:
		return false, "last fetch under 12 hours ago"
	}
	switch Stage(DatesOf(t), now) {
	case StageAnnounced:
		return true, "announced"
	case StageInCinemas:
		return true, "in cinemas"
	}
	if p := t.PhysicalDate; p != nil && !now.Before(*p) && now.Sub(*p) <= physicalWindow {
		return true, "physical release in the last 30 days"
	}
	return false, "released"
}

// changedSince is the time the changed list is asked from: the last start
// rounded down to the hour, minus 15 minutes.
func changedSince(lastStart time.Time) time.Time {
	return lastStart.UTC().Truncate(time.Hour).Add(-15 * time.Minute)
}

// RefreshPlan picks the Titles a refresh-all run covers. byHand covers every
// movie. Otherwise, when the last start is under 14 days ago and the
// provider has a changed list, the movies on it are refreshed; when that
// list is empty or unavailable the rules decide (ShouldRefresh). Either
// way, up to ScoreFillPerRun movies with no score yet are added.
func (s *Service) RefreshPlan(ctx context.Context, byHand bool, lastStart *time.Time) (ids []int64, why map[int64]string, err error) {
	ts, err := database.Select[mediafiles.Title](ctx, s.Store.Read(), "titles", "kind = 'movie' AND state != 'removed' ORDER BY id")
	if err != nil {
		return nil, nil, err
	}
	why = map[int64]string{}
	now := s.now()
	if byHand {
		for _, t := range ts {
			ids, why[t.ID] = append(ids, t.ID), "by hand"
		}
		return ids, why, nil
	}
	var changed map[string]bool
	if cl, ok := s.Metadata.(metadata.ChangeLister); ok && lastStart != nil && now.Sub(*lastStart) < changedListAge {
		list, err := cl.Changed(ctx, changedSince(*lastStart))
		switch {
		case err == nil:
			changed = map[string]bool{}
			for _, id := range list {
				changed[id] = true
			}
		case errors.Is(err, metadata.ErrNoChangeList):
		default:
			log.Printf("library: changed list: %v; using the refresh rules", err)
		}
	}
	filled := 0
	for _, t := range ts {
		if filled < ScoreFillPerRun && NeedsScore(t, now, refreshFresh) {
			ids, why[t.ID], filled = append(ids, t.ID), "no score yet", filled+1
			continue
		}
		if len(changed) > 0 {
			if changed[lookupID(t)] {
				ids, why[t.ID] = append(ids, t.ID), "on the changed list"
			}
			continue
		}
		if yes, w := ShouldRefresh(t, now); yes {
			ids, why[t.ID] = append(ids, t.ID), w
		}
	}
	return ids, why, nil
}

// handleRefreshTitles refreshes the planned Titles one by one. A failure
// on one movie is recorded and the run goes on.
func (s *Service) handleRefreshTitles(ctx context.Context, r *jobs.Run) error {
	byHand, _ := r.Job.Params["by_hand"].(bool)
	scheduled, _ := r.Job.Params["scheduled"].(bool)
	if scheduled && !byHand {
		on, err := s.movieRefreshScheduleOn(ctx)
		if err != nil {
			return err
		}
		if !on {
			r.SetResult(map[string]any{"refreshed": 0, "failed": []string{}, "dropped": 0, "stopped": "schedule_off"})
			return nil
		}
	}
	var last *time.Time
	if v, _ := r.Job.Params["last_start"].(string); v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			last = &t
		}
	}
	ids, why, err := s.RefreshPlan(ctx, byHand, last)
	if err != nil {
		return err
	}
	var failed []string
	dropped := 0
	for i, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if scheduled && !byHand {
			on, err := s.movieRefreshScheduleOn(ctx)
			if err != nil {
				return err
			}
			if !on {
				r.SetResult(map[string]any{"refreshed": i - len(failed), "failed": failed, "dropped": dropped, "stopped": "schedule_off"})
				return nil
			}
		}
		_ = r.Progress(ctx, float64(i)/float64(len(ids)), fmt.Sprintf("refreshing %d of %d", i+1, len(ids)))
		out, err := s.RefreshTitle(ctx, id, JobActor(r), &r.Job.ID)
		if err != nil {
			if errors.Is(err, metadata.ErrNoAccess) {
				return err // every other movie would fail the same way
			}
			failed = append(failed, fmt.Sprintf("title %d: %v", id, err))
			continue
		}
		if out.Dropped {
			dropped++
		}
	}
	reasons := map[string]int{}
	for _, w := range why {
		reasons[w]++
	}
	r.SetResult(map[string]any{"refreshed": len(ids) - len(failed), "failed": failed, "dropped": dropped, "why": reasons})
	return nil
}

func (s *Service) movieRefreshScheduleOn(ctx context.Context) (bool, error) {
	var enabled bool
	err := s.Store.Read().QueryRowContext(ctx, "SELECT enabled FROM schedules WHERE kind = ?", KindRefreshTitles).Scan(&enabled)
	return enabled, err
}

// OpenGates moves Slots whose gate has opened from not_out_yet to wanted.
// It reads first and writes only when a Slot crosses, so an idle Library
// causes no disk writes.
func (s *Service) OpenGates(ctx context.Context) (int, error) {
	now := s.now()
	ids, err := database.Select[mediafiles.Title](ctx, s.Store.Read(), "titles", `state != 'removed' AND (gate_driver = 'announced' OR (gate_opens_at IS NOT NULL AND gate_opens_at <= ?))
		AND id IN (SELECT i.title_id FROM items i JOIN slots sl ON sl.item_id = i.id WHERE sl.state = 'not_out_yet')`, database.FormatTime(now))
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	n := 0
	for _, t := range ids {
		var opened []int64
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			var err error
			opened, _, err = applyGateTx(ctx, tx, t, now, domain.System)
			return err
		})
		if err != nil {
			return n, err
		}
		n += len(opened)
		s.gateEvents(ctx, t, opened, nil, domain.System, nil)
	}
	return n, nil
}

// GatesPass opens the movie, episode and album gates once (the supervisor
// runs it every minute). Each kind runs even when another failed; the
// first error is returned.
func (s *Service) GatesPass(ctx context.Context) error {
	var first error
	for _, g := range []struct {
		what string
		open func(context.Context) (int, error)
	}{{"gates", s.OpenGates}, {"episode gates", s.openEpisodeGates}, {"album gates", s.OpenTrackGates}} {
		if _, err := g.open(ctx); err != nil && ctx.Err() == nil {
			log.Printf("library: %s: %v", g.what, err)
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// CalendarEntry is one release date of a movie in the Library, or one
// episode's air time.
type CalendarEntry struct {
	TitleID int64     `json:"title_id"`
	Name    string    `json:"name"`
	Year    *int      `json:"year,omitempty"`
	Kind    string    `json:"kind"` // cinema, digital, physical, episode
	Date    time.Time `json:"date"`
	// An episode: Date is the exact air time (UTC) when Timed, else the
	// local air day at midnight UTC; AirDay is the day where it airs.
	ItemID      int64  `json:"item_id,omitempty"`
	Season      *int   `json:"season,omitempty"`
	Episode     *int   `json:"episode,omitempty"`
	EpisodeName string `json:"episode_name,omitempty"`
	// TMDB is the show's or movie's TMDB id, for reading its description.
	TMDB    string `json:"tmdb,omitempty"`
	AirDay  string `json:"air_day,omitempty"`
	Timed   bool   `json:"timed"`
	Runtime int    `json:"runtime,omitempty"` // minutes
	Network string `json:"network,omitempty"`
	// State is the main Version's Slot state of the episode.
	State        string `json:"state,omitempty"`
	KeepUpToDate bool   `json:"keep_up_to_date,omitempty"`
	// Versions are every Version's Slot state of the episode (main first),
	// only when the show has more than one Version.
	Versions []VersionState `json:"versions,omitempty"`
	// An album: Name is the artist; the album, its kind, and how many of its
	// tracks are in the Library.
	PartID    int64  `json:"part_id,omitempty"`
	Album     string `json:"album,omitempty"`
	AlbumType string `json:"album_type,omitempty"`
	Tracks    int    `json:"tracks,omitempty"`
	Have      int    `json:"have,omitempty"`
	// Pictures: the title's poster (stored artwork, the API turns it into a
	// link) and wide backdrop, and an episode's still when known.
	Poster   string `json:"poster,omitempty"`
	Backdrop string `json:"backdrop,omitempty"`
	Still    string `json:"still,omitempty"`
	// A movie, for the words under its card: genres, length (Runtime),
	// directors, the top-billed cast, the movie series it belongs to, and
	// its cinema day and first day out to watch at home (digital or disc).
	Genres     []string   `json:"genres,omitempty"`
	Directors  []string   `json:"directors,omitempty"`
	Cast       []string   `json:"cast,omitempty"`
	Collection string     `json:"collection,omitempty"`
	Studio     string     `json:"studio,omitempty"`
	CinemaDate *time.Time `json:"cinema_date,omitempty"`
	HomeDate   *time.Time `json:"home_date,omitempty"`
	// An album: its length in minutes (every track with a known length),
	// and its place among the artist's releases of the same kind (1 for the
	// first album; extra kinds such as live or compilation are not counted).
	LengthMin int `json:"length_min,omitempty"`
	Nth       int `json:"nth,omitempty"`
	// ArtistMBID lets the API add the artist's genres it already knows.
	ArtistMBID string `json:"-"`
}

// maxCalendarCast is how many top-billed names a movie's date carries.
const maxCalendarCast = 3

// creditsOf splits a title's people into directors and the top-billed cast.
// Titles not refreshed since directors were kept on their own still list
// them after a full cast (the metadata keeps 10 cast names), so names past
// the tenth are read as directors.
func creditsOf(t mediafiles.Title) (directors, cast []string) {
	directors, people := t.Directors, t.People
	if len(directors) == 0 && len(people) > 10 {
		directors, people = people[10:], people[:10]
	}
	for _, p := range people {
		if len(cast) == maxCalendarCast {
			break
		}
		if !slices.Contains(directors, p) {
			cast = append(cast, p)
		}
	}
	return directors, cast
}

// movieDetails fills a movie date's words from its title and series.
func movieDetails(e *CalendarEntry, t mediafiles.Title, series map[int64]string) {
	e.Genres = t.Genres
	e.Runtime = t.RuntimeMinutes
	e.Directors, e.Cast = creditsOf(t)
	e.Collection = series[t.ID]
	e.Studio = t.Studio
	e.CinemaDate = t.CinemaDate
	for _, d := range []*time.Time{t.DigitalDate, t.PhysicalDate} {
		if d != nil && (e.HomeDate == nil || d.Before(*e.HomeDate)) {
			e.HomeDate = d
		}
	}
}

// movieSeries names the movie series each title belongs to.
func movieSeries(ctx context.Context, db Reader, ids []any) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT ct.title_id, c.name FROM movie_collection_titles ct
		JOIN movie_collections c ON c.id = ct.collection_id WHERE ct.title_id IN (`+marks(len(ids))+`) ORDER BY c.id`, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		if out[id] == "" {
			out[id] = name
		}
	}
	return out, rows.Err()
}

// posterOf is a title's first stored picture ("" when it has none).
func posterOf(t mediafiles.Title) string {
	if len(t.Artwork) > 0 {
		return t.Artwork[0]
	}
	return ""
}

// VersionState is one Version's Slot state of an episode.
type VersionState struct {
	Label string `json:"label"`
	State string `json:"state"`
}

// Which titles a calendar shows.
const (
	CalendarAll    = "all"
	CalendarMovies = "movies"
	CalendarShows  = "shows"
	CalendarMusic  = "music"
)

// Calendar lists the cinema, digital and physical dates in [from, to) of
// movies in the Library, the episodes of its shows airing then and the
// release days of its artists' albums, by date, then cinema, digital,
// physical, episode, album. titles narrows it to movies, shows or music
// ("" or all: every one).
func Calendar(ctx context.Context, db Reader, from, to time.Time, titles ...string) ([]CalendarEntry, error) {
	which := CalendarAll
	if len(titles) > 0 && titles[0] != "" {
		which = titles[0]
	}
	order := map[string]int{"cinema": 0, "digital": 1, "physical": 2, "episode": 3, "album": 4}
	out := []CalendarEntry{}
	if which == CalendarAll || which == CalendarShows {
		eps, err := CalendarEpisodes(ctx, db, from, to)
		if err != nil {
			return nil, err
		}
		out = append(out, eps...)
	}
	if which == CalendarAll || which == CalendarMusic {
		als, err := calendarAlbums(ctx, db, from, to)
		if err != nil {
			return nil, err
		}
		out = append(out, als...)
	}
	if which == CalendarShows || which == CalendarMusic {
		sortEntries(out, order)
		return out, nil
	}
	f, t := database.FormatTime(from), database.FormatTime(to)
	ts, err := database.Select[mediafiles.Title](ctx, db, "titles", `kind = 'movie' AND state != 'removed' AND (
		(cinema_date >= ? AND cinema_date < ?) OR (digital_date >= ? AND digital_date < ?) OR (physical_date >= ? AND physical_date < ?))`,
		f, t, f, t, f, t)
	if err != nil {
		return nil, err
	}
	ids := make([]any, len(ts))
	for i, ti := range ts {
		ids[i] = ti.ID
	}
	series, err := movieSeries(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	for _, ti := range ts {
		for _, d := range []struct {
			kind string
			at   *time.Time
		}{{"cinema", ti.CinemaDate}, {"digital", ti.DigitalDate}, {"physical", ti.PhysicalDate}} {
			if d.at != nil && !d.at.Before(from) && d.at.Before(to) {
				e := CalendarEntry{TitleID: ti.ID, Name: ti.Name, Year: ti.Year, Kind: d.kind, Date: *d.at, TMDB: ti.SourceIDs["tmdb"],
					Poster: posterOf(ti), Backdrop: ti.Backdrop}
				movieDetails(&e, ti, series)
				out = append(out, e)
			}
		}
	}
	sortEntries(out, order)
	return out, nil
}

func sortEntries(es []CalendarEntry, order map[string]int) {
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		if a.Kind != b.Kind {
			return order[a.Kind] < order[b.Kind]
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Season != nil && b.Season != nil && *a.Season != *b.Season {
			return *a.Season < *b.Season
		}
		return a.Episode != nil && b.Episode != nil && *a.Episode < *b.Episode
	})
}

// calendarAlbums lists the included albums of artists in the Library whose
// release day falls in [from, to), each with how many of its tracks are in
// the Library.
func calendarAlbums(ctx context.Context, db Reader, from, to time.Time) ([]CalendarEntry, error) {
	// An album known only by month or year has no release day: left out.
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", `kind = 'album' AND included = 1 AND dropped_at IS NULL
		AND coalesce(release_date_precision, 'day') = 'day'
		AND release_date >= ? AND release_date < ? AND title_id IN (SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed') ORDER BY id`,
		database.FormatTime(from), database.FormatTime(to))
	if err != nil {
		return nil, err
	}
	artists := map[int64]mediafiles.Title{}
	out := []CalendarEntry{}
	for _, p := range parts {
		t, seen := artists[p.TitleID]
		if !seen {
			if err := database.Get(ctx, db, "titles", p.TitleID, &t); err != nil {
				return nil, err
			}
			artists[p.TitleID] = t
		}
		e := CalendarEntry{TitleID: t.ID, Name: t.Name, Kind: "album", Date: p.ReleaseDate.UTC(), PartID: p.ID, Album: p.Name, KeepUpToDate: p.KeepUpToDate,
			Poster: posterOf(t), Backdrop: t.Backdrop}
		if p.ReleaseType != nil {
			e.AlbumType = *p.ReleaseType
		}
		y := p.ReleaseDate.Year()
		e.Year = &y
		var ms int64
		_ = db.QueryRowContext(ctx, `SELECT count(*), count(s.file_id), coalesce(sum(i.length_ms), 0) FROM items i LEFT JOIN slots s ON s.item_id = i.id
			AND s.version_id = (SELECT id FROM versions WHERE title_id = i.title_id AND is_main = 1)
			WHERE i.part_id = ? AND i.kind = 'track' AND i.dropped_at IS NULL`, p.ID).Scan(&e.Tracks, &e.Have, &ms)
		e.LengthMin = int((ms + 30000) / 60000)
		e.ArtistMBID = strings.ToLower(t.SourceIDs["musicbrainz"])
		// Its place among the artist's releases of the same kind, by day
		// (a plain album, EP or single; live, compilation and the like are not counted).
		if len(p.SecondaryTypes) == 0 {
			_ = db.QueryRowContext(ctx, `SELECT 1 + count(*) FROM parts WHERE title_id = ? AND kind = 'album' AND dropped_at IS NULL
				AND coalesce(release_type, '') = coalesce(?, '') AND coalesce(secondary_types, '[]') = '[]'
				AND release_date IS NOT NULL AND release_date < ? AND id != ?`, p.TitleID, p.ReleaseType, database.FormatTime(*p.ReleaseDate), p.ID).Scan(&e.Nth)
		}
		out = append(out, e)
	}
	return out, nil
}

// CalendarEpisodes lists the episodes of shows in the Library airing in
// [from, to): by their exact air time, or by their air day when the time
// is unknown. An air day is a local date: it is listed when that day, in
// from's time zone, overlaps the range. The query reads a day either side
// and the exact test is made here.
func CalendarEpisodes(ctx context.Context, db Reader, from, to time.Time) ([]CalendarEntry, error) {
	items, err := database.Select[mediafiles.Item](ctx, db, "items", `kind = 'episode' AND dropped_at IS NULL
		AND title_id IN (SELECT id FROM titles WHERE kind = 'show' AND state != 'removed')
		AND ((air_timed = 1 AND air_date >= ? AND air_date < ?) OR ((air_timed = 0 OR air_date IS NULL) AND air_day >= ? AND air_day <= ?)) ORDER BY id`,
		database.FormatTime(from), database.FormatTime(to), from.AddDate(0, 0, -1).Format("2006-01-02"), to.AddDate(0, 0, 1).Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	// The shows and the Slots of every episode, one query each for the
	// whole range rather than per episode.
	shows := map[int64]mediafiles.Title{}
	states := map[int64][]VersionState{} // item -> its Slots, main Version first
	if len(items) > 0 {
		var titleIDs, itemIDs []any
		seen := map[int64]bool{}
		for _, it := range items {
			itemIDs = append(itemIDs, it.ID)
			if !seen[it.TitleID] {
				seen[it.TitleID] = true
				titleIDs = append(titleIDs, it.TitleID)
			}
		}
		ts, err := database.Select[mediafiles.Title](ctx, db, "titles", "id IN ("+marks(len(titleIDs))+")", titleIDs...)
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			shows[t.ID] = t
		}
		rows, err := db.QueryContext(ctx, `SELECT s.item_id, v.label, s.state FROM slots s JOIN versions v ON v.id = s.version_id
			WHERE s.item_id IN (`+marks(len(itemIDs))+`) ORDER BY s.item_id, v.is_main DESC, v.id`, itemIDs...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var v VersionState
			if err := rows.Scan(&id, &v.Label, &v.State); err != nil {
				_ = rows.Close()
				return nil, err
			}
			states[id] = append(states[id], v)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	out := []CalendarEntry{}
	for _, it := range items {
		t, ok := shows[it.TitleID]
		if !ok {
			return nil, database.ErrNotFound
		}
		e := CalendarEntry{TitleID: t.ID, Name: t.Name, Year: t.Year, Kind: "episode", ItemID: it.ID, Season: it.Numbers.Season, Episode: it.Numbers.Episode,
			KeepUpToDate: it.KeepUpToDate, Poster: posterOf(t), Backdrop: t.Backdrop, Still: it.Still, TMDB: t.SourceIDs["tmdb"]}
		if it.Name != nil && !strings.EqualFold(*it.Name, "TBA") {
			e.EpisodeName = *it.Name
		}
		if it.AirDay != nil {
			e.AirDay = *it.AirDay
		}
		if it.Runtime != nil {
			e.Runtime = *it.Runtime
		}
		if t.Network != nil {
			e.Network = *t.Network
		}
		switch {
		case it.AirDate != nil && it.AirTimed:
			e.Date, e.Timed = it.AirDate.UTC(), true
			if e.AirDay == "" {
				e.AirDay = e.Date.Format("2006-01-02")
			}
		default:
			// Only the day is known: an all-day entry on it (an untimed
			// AirDate is the day's end, never a time to show).
			if e.AirDay == "" && it.AirDate != nil {
				e.AirDay = it.AirDate.UTC().Add(-time.Second).Format("2006-01-02")
			}
			d, err := time.Parse("2006-01-02", e.AirDay)
			if err != nil {
				continue
			}
			start := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, from.Location())
			if !start.Before(to) || !start.AddDate(0, 0, 1).After(from) {
				continue // the day is outside the range
			}
			e.Date = d
		}
		if vs := states[it.ID]; len(vs) > 0 {
			e.State = vs[0].State
			if len(vs) > 1 {
				e.Versions = vs
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// marks is n SQL placeholders, comma separated.
func marks(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }
