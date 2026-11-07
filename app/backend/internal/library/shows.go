package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Season choices: which seasons are kept up to date when a show is added.
const (
	SeasonsAll    = "all"
	SeasonsFuture = "future"
	SeasonsLatest = "latest"
	SeasonsNone   = "none"
)

// Episode orders a show can follow.
const (
	OrderAired    = "aired"
	OrderDaily    = "daily"
	OrderAbsolute = "absolute"
)

// Errors about shows.
var (
	ErrBadSeasonChoice = errors.New("seasons must be all, future, latest or none")
	ErrBadNewSeasons   = errors.New("new_seasons must be all or none")
	ErrBadOrder        = errors.New("episode_order must be aired, daily or absolute")
	ErrBadWait         = errors.New("episode_wait_minutes must be between 0 and 10080")
	ErrNotShow         = errors.New("that Title is not a show")
	ErrShowOnly        = errors.New("that choice is for shows only")
	ErrMovieOnly       = errors.New("the Release gate is for movies; shows are searched once each episode has aired")
	ErrNoShowFolder    = errors.New("no show Library folder")
	ErrNotShowFolder   = errors.New("that Library folder is not for shows")
	ErrNoShowSources   = errors.New("show details sources are not set up")
	ErrNoSuchSeason    = errors.New("the show has no such season")
)

// ValidSeasonChoice reports whether c is a season choice.
func ValidSeasonChoice(c string) bool {
	return c == SeasonsAll || c == SeasonsFuture || c == SeasonsLatest || c == SeasonsNone
}

// ValidOrder reports whether o is an episode order HolyArrs follows.
func ValidOrder(o string) bool { return o == OrderAired || o == OrderDaily || o == OrderAbsolute }

// ShowDefaults are the show settings (What to pick).
type ShowDefaults struct {
	SeasonChoice string `json:"season_choice"` // all, future, latest, none
	NewSeasons   string `json:"new_seasons"`   // all, none
	EpisodeOrder string `json:"episode_order"` // aired, daily, absolute
	Source       string `json:"details_source"`
	KeepSpecials bool   `json:"keep_specials"`
	WaitMinutes  int    `json:"episode_wait_minutes"` // after the air time before an episode is wanted
}

// ReadShowDefaults reads the show settings; unset or odd values take the
// recommended defaults (all seasons, new seasons kept, aired order, TMDB,
// specials off, no wait).
func ReadShowDefaults(ctx context.Context, db store.Lister) (ShowDefaults, error) {
	d := ShowDefaults{SeasonChoice: SeasonsAll, NewSeasons: "all", EpisodeOrder: OrderAired, Source: "tmdb"}
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings WHERE grp = 'what_to_pick' AND key IN
		('show_season_choice', 'show_new_seasons', 'show_episode_order', 'show_details_source', 'keep_specials', 'episode_wait_minutes')`)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return d, err
		}
		switch k {
		case "show_season_choice":
			if ValidSeasonChoice(v) {
				d.SeasonChoice = v
			}
		case "show_new_seasons":
			if v == "all" || v == "none" {
				d.NewSeasons = v
			}
		case "show_episode_order":
			if ValidOrder(v) {
				d.EpisodeOrder = v
			}
		case "show_details_source":
			if metadata.ValidShowSource(v) {
				d.Source = v
			}
		case "keep_specials":
			d.KeepSpecials = v == "true"
		case "episode_wait_minutes":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 10080 {
				d.WaitMinutes = n
			}
		}
	}
	return d, rows.Err()
}

// SetShowDefaults stores the show settings.
func SetShowDefaults(ctx context.Context, s *store.Store, d ShowDefaults, by domain.Actor) error {
	switch {
	case !ValidSeasonChoice(d.SeasonChoice):
		return ErrBadSeasonChoice
	case d.NewSeasons != "all" && d.NewSeasons != "none":
		return ErrBadNewSeasons
	case !ValidOrder(d.EpisodeOrder):
		return ErrBadOrder
	case !metadata.ValidShowSource(d.Source):
		return metadata.ErrUnknownSource
	case d.WaitMinutes < 0 || d.WaitMinutes > 10080:
		return ErrBadWait
	}
	vals := map[string]string{"show_season_choice": d.SeasonChoice, "show_new_seasons": d.NewSeasons, "show_episode_order": d.EpisodeOrder,
		"show_details_source": d.Source, "keep_specials": strconv.FormatBool(d.KeepSpecials), "episode_wait_minutes": strconv.Itoa(d.WaitMinutes)}
	return s.Write(ctx, func(tx *sql.Tx) error {
		now := store.FormatTime(time.Now())
		for k, v := range vals {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, v, now, actorText(by), now, actorText(by))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// EpisodeGate says whether an episode that airs at air is out at now,
// counting wait after the air time, and why in one phrase. An episode with
// no air date is never out.
func EpisodeGate(air *time.Time, wait time.Duration, now time.Time) (open bool, reason string) {
	if air == nil {
		return false, "no air date yet"
	}
	from := air.Add(wait)
	if now.Before(from) {
		if wait > 0 {
			return false, fmt.Sprintf("airs %s UTC; wanted from %s UTC", air.UTC().Format("2006-01-02 15:04"), from.UTC().Format("2006-01-02 15:04"))
		}
		return false, "airs " + air.UTC().Format("2006-01-02 15:04") + " UTC"
	}
	return true, "aired " + air.UTC().Format("2006-01-02 15:04") + " UTC"
}

// showSources returns the sources, or an error when there are none.
func (s *Service) showSource(name string) (metadata.ShowProvider, error) {
	if s.Shows == nil {
		return nil, ErrNoShowSources
	}
	return s.Shows.Get(name)
}

// idOrder is the order other ids are tried in when a source has no id of
// its own for a show yet.
var idOrder = []string{"tvdb", "imdb", "tmdb", "tvmaze"}

// lookupShow fetches a show from source using ids: the source's own id
// first, then the other known ids. ErrShowNotFound when none of them work.
func lookupShow(ctx context.Context, p metadata.ShowProvider, source string, ids map[string]string) (metadata.Show, error) {
	tried := map[string]bool{}
	for _, k := range append([]string{source}, idOrder...) {
		v := ids[k]
		if v == "" || tried[k] {
			continue
		}
		tried[k] = true
		sh, err := p.Show(ctx, k+":"+v)
		if errors.Is(err, metadata.ErrShowNotFound) {
			continue
		}
		return sh, err
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

// addAirTimes gives a show from a source that lists only air days (TMDB,
// TVDB) the exact air times TVMaze lists for the same show. The TVMaze show
// is found by its TVMaze id when known, else by the TVDB or IMDb id, else
// by name with a TVDB or IMDb id that must match. Best effort: when TVMaze
// is unreachable or does not know the show, the episodes stay untimed.
func (s *Service) addAirTimes(ctx context.Context, sh *metadata.Show) {
	if sh.Source == "tvmaze" || len(sh.Episodes) == 0 || s.Shows == nil {
		return
	}
	untimed := false
	for _, e := range sh.Episodes {
		if !e.Timed && e.AirDay != "" {
			untimed = true
			break
		}
	}
	if !untimed {
		return
	}
	p, err := s.Shows.Get("tvmaze")
	if err != nil || p == nil {
		return
	}
	var maze metadata.Show
	found := false
	for _, k := range []string{"tvmaze", "tvdb", "imdb"} {
		v := sh.IDs[k]
		if v == "" {
			continue
		}
		m, err := p.Show(ctx, k+":"+v)
		if err == nil && (k == "tvmaze" || metadata.SameShow(*sh, m)) {
			maze, found = m, true
			break
		}
		if err != nil && !errors.Is(err, metadata.ErrShowNotFound) {
			return // TVMaze is unreachable: try again on the next refresh
		}
	}
	if !found && sh.Name != "" {
		hits, err := p.SearchShows(ctx, sh.Name)
		if err != nil {
			return
		}
		for _, h := range hits {
			if metadata.SameShow(*sh, h) && h.IDs["tvmaze"] != "" {
				if m, err := p.Show(ctx, "tvmaze:"+h.IDs["tvmaze"]); err == nil {
					maze, found = m, true
				}
				break
			}
		}
	}
	if found {
		metadata.AddAirTimes(sh, maze)
	}
}

// applyShow copies looked-up details onto a show Title.
func applyShow(t *domain.Title, sh metadata.Show) {
	t.Name, t.SortName = sh.Name, sortName(sh.Name)
	ids := map[string]string{}
	for k, v := range t.SourceIDs {
		ids[k] = v
	}
	for k, v := range sh.IDs {
		if v != "" {
			ids[k] = v
		}
	}
	t.SourceIDs = ids
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	t.Year = nil
	if sh.Year > 0 {
		y := sh.Year
		t.Year = &y
	}
	t.Overview, t.OriginalLanguage = str(sh.Overview), str(sh.OriginalLanguage)
	t.Network, t.AirTime, t.TimeZone = str(sh.Network), str(sh.AirTime), str(sh.TimeZone)
	t.Aliases = mergeAliases(t.Aliases, sh.Name, sh.OtherNames)
	switch sh.Status {
	case "ended", "announced", "running":
		t.Status = sh.Status
	default:
		t.Status = "running"
	}
	applyDetails(t, sh.Backdrop, sh.Logo, sh.Genres, sh.Rating, sh.People, sh.Runtime)
}

// absolutes gives each regular episode its absolute number: the source's,
// when it gives any, else the count in aired order. Specials get none.
func absolutes(eps []metadata.Episode) map[[2]int]int {
	out := map[[2]int]int{}
	given := false
	for _, e := range eps {
		if e.Season > 0 && e.Absolute > 0 {
			given = true
		}
	}
	reg := make([]metadata.Episode, 0, len(eps))
	for _, e := range eps {
		if e.Season > 0 {
			reg = append(reg, e)
		}
	}
	sort.SliceStable(reg, func(i, j int) bool {
		if reg[i].Season != reg[j].Season {
			return reg[i].Season < reg[j].Season
		}
		return reg[i].Number < reg[j].Number
	})
	for i, e := range reg {
		switch {
		case given && e.Absolute > 0:
			out[[2]int{e.Season, e.Number}] = e.Absolute
		case !given:
			out[[2]int{e.Season, e.Number}] = i + 1
		}
	}
	return out
}

// ShowSync says what one sync of a show's seasons and episodes did.
type ShowSync struct {
	SeasonsAdded    int     `json:"seasons_added"`
	EpisodesAdded   int     `json:"episodes_added"`
	EpisodesChanged int     `json:"episodes_changed"`
	EpisodesRemoved int     `json:"episodes_removed"`
	EpisodesDropped int     `json:"episodes_dropped"` // kept because a file or download holds them
	Opened          []int64 `json:"gate_opened_slots,omitempty"`
	Closed          []int64 `json:"gate_closed_slots,omitempty"`
}

// seasonKeep says whether a season starts kept up to date. choice is the
// add-time season choice; "" means a season a later refresh found, which
// follows the show's new-seasons policy.
func seasonKeep(t domain.Title, number, latest int, choice string, keepSpecials bool) bool {
	if !t.KeepUpToDate {
		return false
	}
	if number == 0 {
		return keepSpecials && (choice == SeasonsAll || choice == SeasonsFuture || (choice == "" && t.NewPartsPolicy == "all"))
	}
	switch choice {
	case SeasonsAll, SeasonsFuture:
		return true
	case SeasonsLatest:
		return number == latest
	case SeasonsNone:
		return false
	}
	return t.NewPartsPolicy == "all"
}

// futureCut is the moment before which an episode is not wanted: now when
// a show is added with Future seasons only, else its stored future_from.
func futureCut(t domain.Title, choice string, now time.Time) *time.Time {
	if choice == SeasonsFuture {
		return &now
	}
	if choice == "" {
		return t.FutureFrom
	}
	return nil
}

// activeStates are Slot states where a Download or File holds the episode.
const holdingSlots = `file_id IS NOT NULL OR active_download_id IS NOT NULL OR held_release_id IS NOT NULL OR state IN ('searching', 'held', 'chosen', 'downloading', 'stalled', 'importing', 'needs_a_decision', 'in_library', 'better_wanted')`

// syncShowTx brings a show's seasons, episodes and Slots in line with sh.
// choice is the season choice when the show is being added, "" on a
// refresh. Episodes are matched by the source's own episode id first, then
// by (season, episode), so rows, Slots and Files survive a refresh, a
// renumbered special and a change of details source.
func syncShowTx(ctx context.Context, tx *sql.Tx, t domain.Title, sh metadata.Show, defs ShowDefaults, choice string, now time.Time, by domain.Actor) (ShowSync, error) {
	var out ShowSync
	parts, err := store.Select[domain.Part](ctx, tx, "parts", "title_id = ? AND kind = 'season'", t.ID)
	if err != nil {
		return out, err
	}
	partBy := map[int]*domain.Part{}
	for i := range parts {
		if parts[i].Number != nil {
			partBy[*parts[i].Number] = &parts[i]
		}
	}
	latest := 0
	for _, se := range sh.Seasons {
		latest = max(latest, se.Number)
	}
	for _, e := range sh.Episodes {
		latest = max(latest, e.Season)
	}
	seasonName := map[int]string{}
	for _, se := range sh.Seasons {
		seasonName[se.Number] = se.Name
	}
	ensurePart := func(n int) (*domain.Part, error) {
		if p, ok := partBy[n]; ok {
			changed := p.DroppedAt != nil
			p.DroppedAt = nil
			if nm := seasonName[n]; nm != "" && nm != p.Name {
				p.Name, changed = nm, true
			}
			if changed {
				if err := store.Update(tx, "parts", p, by); err != nil {
					return nil, err
				}
			}
			return p, nil
		}
		name := seasonName[n]
		if name == "" {
			name = "Season " + strconv.Itoa(n)
			if n == 0 {
				name = "Specials"
			}
		}
		num := n
		p := &domain.Part{TitleID: t.ID, Kind: "season", Number: &num, Name: name, KeepUpToDate: seasonKeep(t, n, latest, choice, defs.KeepSpecials),
			SourceIDs: map[string]string{}, Pressings: []domain.Pressing{}}
		if _, err := store.Insert(tx, "parts", p, by); err != nil {
			return nil, err
		}
		partBy[n] = p
		out.SeasonsAdded++
		return p, nil
	}
	for _, se := range sh.Seasons {
		if _, err := ensurePart(se.Number); err != nil {
			return out, err
		}
	}
	items, err := store.Select[domain.Item](ctx, tx, "items", "title_id = ? AND kind = 'episode'", t.ID)
	if err != nil {
		return out, err
	}
	itemBy := map[[2]int]*domain.Item{}
	for i := range items {
		n := items[i].Numbers
		if n.Season != nil && n.Episode != nil {
			itemBy[[2]int{*n.Season, *n.Episode}] = &items[i]
		}
	}
	versions, err := store.Select[domain.Version](ctx, tx, "versions", "title_id = ?", t.ID)
	if err != nil {
		return out, err
	}
	abs := absolutes(sh.Episodes)
	wait := time.Duration(defs.WaitMinutes) * time.Minute
	seen := map[[2]int]bool{}
	var eps []metadata.Episode
	listedIDs := map[string]bool{}
	for _, e := range sh.Episodes {
		key := [2]int{e.Season, e.Number}
		if seen[key] || e.Number <= 0 {
			continue // a source listing an episode twice keeps the first
		}
		seen[key] = true
		eps = append(eps, e)
		if e.ID != "" {
			listedIDs[e.ID] = true
		}
	}
	// Match rows to the source's episodes: by the source's episode id first
	// (specials are numbered by air day, so a new one shifts the others),
	// then by (season, episode) for a row with no id from this source or
	// one the source no longer lists.
	idBy := map[string]*domain.Item{}
	for i := range items {
		if v := items[i].SourceIDs[sh.Source]; v != "" {
			idBy[v] = &items[i]
		}
	}
	match := map[[2]int]*domain.Item{}
	claimed := map[int64]bool{}
	for _, e := range eps {
		if it, ok := idBy[e.ID]; ok && e.ID != "" && !claimed[it.ID] {
			match[[2]int{e.Season, e.Number}], claimed[it.ID] = it, true
		}
	}
	for _, e := range eps {
		key := [2]int{e.Season, e.Number}
		if match[key] != nil {
			continue
		}
		it, ok := itemBy[key]
		if !ok || claimed[it.ID] || listedIDs[it.SourceIDs[sh.Source]] {
			continue
		}
		match[key], claimed[it.ID] = it, true
	}
	// Episodes the source no longer lists: removed when nothing holds them,
	// else kept and flagged. A kept one whose numbers a listed episode now
	// uses is parked on a negative episode number to free them.
	for i := range items {
		it := &items[i]
		n := it.Numbers
		if claimed[it.ID] || n.Season == nil || n.Episode == nil {
			continue
		}
		var held int
		if err := tx.QueryRow("SELECT count(*) FROM slots WHERE item_id = ? AND ("+holdingSlots+")", it.ID).Scan(&held); err != nil {
			return out, err
		}
		if held == 0 {
			if _, err := tx.Exec("DELETE FROM items WHERE id = ?", it.ID); err != nil {
				return out, err
			}
			out.EpisodesRemoved++
			continue
		}
		park := seen[[2]int{*n.Season, *n.Episode}]
		if it.DroppedAt == nil || park {
			if it.DroppedAt == nil {
				d := now
				it.DroppedAt = &d
				out.EpisodesDropped++
			}
			if park {
				neg := -int(it.ID)
				it.Numbers.Episode = &neg
			}
			if err := store.Update(tx, "items", it, by); err != nil {
				return out, err
			}
		}
	}
	// Rows moving to new numbers are parked first, so two rows trading
	// numbers never clash on the one-row-per-episode index.
	for key, it := range match {
		if *it.Numbers.Season != key[0] || *it.Numbers.Episode != key[1] {
			neg := -int(it.ID)
			it.Numbers.Episode = &neg
			if err := store.Update(tx, "items", it, by); err != nil {
				return out, err
			}
		}
	}
	for _, e := range eps {
		key := [2]int{e.Season, e.Number}
		p, err := ensurePart(e.Season)
		if err != nil {
			return out, err
		}
		var name, day *string
		if e.Name != "" {
			nm := e.Name
			name = &nm
		}
		if e.AirDay != "" {
			d := e.AirDay
			day = &d
		}
		var runtime *int
		if e.Runtime > 0 {
			r := e.Runtime
			runtime = &r
		}
		var absolute *int
		if a, ok := abs[key]; ok {
			absolute = &a
		}
		if it := match[key]; it != nil {
			ids := map[string]string{}
			for k, v := range it.SourceIDs {
				ids[k] = v
			}
			if e.ID != "" {
				ids[sh.Source] = e.ID
			}
			changed := !eqStr(it.Name, name) || !eqTime(it.AirDate, e.AirDate) || it.AirTimed != (e.Timed && e.AirDate != nil) || !eqStr(it.AirDay, day) || !eqInt(it.Runtime, runtime) ||
				!eqInt(it.Numbers.Absolute, absolute) || it.Still != e.Still || it.DroppedAt != nil || len(ids) != len(it.SourceIDs) || ids[sh.Source] != it.SourceIDs[sh.Source] ||
				it.PartID == nil || *it.PartID != p.ID || *it.Numbers.Season != e.Season || *it.Numbers.Episode != e.Number
			if !changed {
				continue
			}
			season, number := e.Season, e.Number
			it.Numbers.Season, it.Numbers.Episode = &season, &number
			it.Name, it.AirDate, it.AirDay, it.Runtime, it.Numbers.Absolute, it.DroppedAt, it.SourceIDs, it.PartID = name, e.AirDate, day, runtime, absolute, nil, ids, &p.ID
			it.Still, it.AirTimed = e.Still, e.Timed && e.AirDate != nil
			if err := store.Update(tx, "items", it, by); err != nil {
				return out, err
			}
			out.EpisodesChanged++
			continue
		}
		season, number := e.Season, e.Number
		keep := p.KeepUpToDate
		if cut := futureCut(t, choice, now); cut != nil && e.AirDate != nil && !e.AirDate.After(*cut) {
			keep = false // future: only episodes still to air
		}
		it := &domain.Item{TitleID: t.ID, PartID: &p.ID, Kind: "episode", Numbers: domain.Numbers{Season: &season, Episode: &number, Absolute: absolute},
			Name: name, AirDate: e.AirDate, AirDay: day, AirTimed: e.Timed && e.AirDate != nil, Runtime: runtime, KeepUpToDate: keep, SourceIDs: map[string]string{}, Still: e.Still}
		if e.ID != "" {
			it.SourceIDs[sh.Source] = e.ID
		}
		if _, err := store.Insert(tx, "items", it, by); err != nil {
			return out, err
		}
		out.EpisodesAdded++
		open, reason := EpisodeGate(e.AirDate, wait, now)
		state := domain.SlotNotOutYet
		if open {
			state = domain.SlotWanted
		}
		for _, v := range versions {
			r := reason
			if _, err := store.Insert(tx, "slots", &domain.Slot{ItemID: it.ID, VersionID: v.ID, State: state, LastReason: &r}, by); err != nil {
				return out, err
			}
		}
	}
	if choice == SeasonsFuture {
		// Future seasons only: a season with nothing still to air is not wanted.
		ahead := map[int]bool{}
		listedIn := map[int]bool{}
		for _, e := range eps {
			listedIn[e.Season] = true
			if e.AirDate == nil || e.AirDate.After(now) {
				ahead[e.Season] = true
			}
		}
		for n, p := range partBy {
			if p.KeepUpToDate && listedIn[n] && !ahead[n] {
				p.KeepUpToDate = false
				if err := store.Update(tx, "parts", p, by); err != nil {
					return out, err
				}
			}
		}
	}
	// Seasons the source no longer lists: removed when empty, else flagged.
	listed := map[int]bool{}
	for _, se := range sh.Seasons {
		listed[se.Number] = true
	}
	for key := range seen {
		listed[key[0]] = true
	}
	for n, p := range partBy {
		if listed[n] {
			continue
		}
		var left int
		if err := tx.QueryRow("SELECT count(*) FROM items WHERE part_id = ?", p.ID).Scan(&left); err != nil {
			return out, err
		}
		if left == 0 {
			if _, err := tx.Exec("DELETE FROM parts WHERE id = ?", p.ID); err != nil {
				return out, err
			}
		} else if p.DroppedAt == nil {
			d := now
			p.DroppedAt = &d
			if err := store.Update(tx, "parts", p, by); err != nil {
				return out, err
			}
		}
	}
	out.Opened, out.Closed, err = episodeGatesTx(ctx, tx, t.ID, wait, now, by)
	return out, err
}

func eqStr(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
func eqInt(a, b *int) bool    { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
func eqTime(a, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

// episodeGatesTx moves a show's episode Slots across the air-date gate:
// not_out_yet to wanted once aired (plus the wait), and wanted back to
// not_out_yet when the air date moved later. titleID 0 means every show.
func episodeGatesTx(ctx context.Context, tx *sql.Tx, titleID int64, wait time.Duration, now time.Time, by domain.Actor) (opened, closed []int64, err error) {
	q := `SELECT s.id, s.state, i.air_date, COALESCE(s.last_reason, '') FROM slots s JOIN items i ON i.id = s.item_id
		WHERE i.kind = 'episode' AND s.state IN ('not_out_yet', 'wanted')`
	args := []any{}
	if titleID != 0 {
		q += " AND i.title_id = ?"
		args = append(args, titleID)
	}
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	type row struct {
		id     int64
		state  string
		air    *time.Time
		reason string
	}
	var rs []row
	for rows.Next() {
		var r row
		var air sql.NullString
		if err := rows.Scan(&r.id, &r.state, &air, &r.reason); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if air.Valid {
			if t, err := time.Parse(store.TimeLayout, air.String); err == nil {
				r.air = &t
			} else if t, err := time.Parse(time.RFC3339Nano, air.String); err == nil {
				r.air = &t
			}
		}
		rs = append(rs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for _, r := range rs {
		open, reason := EpisodeGate(r.air, wait, now)
		switch {
		case open && r.state == string(domain.SlotNotOutYet):
			if err := store.MoveSlotTx(tx, r.id, domain.SlotWanted, reason, by); err != nil {
				return nil, nil, err
			}
			opened = append(opened, r.id)
		case !open && r.state == string(domain.SlotWanted):
			if err := store.MoveSlotTx(tx, r.id, domain.SlotNotOutYet, reason, by); err != nil {
				return nil, nil, err
			}
			closed = append(closed, r.id)
		case r.state == string(domain.SlotNotOutYet) && r.reason != reason:
			if _, err := tx.Exec("UPDATE slots SET last_reason = ? WHERE id = ?", reason, r.id); err != nil {
				return nil, nil, err
			}
		}
	}
	return opened, closed, nil
}
