package shows

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// SetShowDefaults stores the show settings.
func SetShowDefaults(ctx context.Context, s *database.Store, d library.ShowDefaults, by domain.Actor) error {
	switch {
	case !library.ValidSeasonChoice(d.SeasonChoice):
		return library.ErrBadSeasonChoice
	case d.NewSeasons != "all" && d.NewSeasons != "none":
		return library.ErrBadNewSeasons
	case !library.ValidOrder(d.EpisodeOrder):
		return library.ErrBadOrder
	case !metadata.ValidShowSource(d.Source):
		return metadata.ErrUnknownSource
	case d.WaitMinutes < 0 || d.WaitMinutes > 10080:
		return library.ErrBadWait
	}
	vals := map[string]string{"show_season_choice": d.SeasonChoice, "show_new_seasons": d.NewSeasons, "show_episode_order": d.EpisodeOrder,
		"show_details_source": d.Source, "keep_specials": strconv.FormatBool(d.KeepSpecials), "episode_wait_minutes": strconv.Itoa(d.WaitMinutes)}
	return s.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now())
		for k, v := range vals {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, v, now, library.ActorText(by), now, library.ActorText(by))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// showSources returns the sources, or an error when there are none.
func (s *Service) showSource(name string) (metadata.ShowProvider, error) {
	if s.Shows == nil {
		return nil, library.ErrNoShowSources
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
func applyShow(t *mediafiles.Title, sh metadata.Show) {
	t.Name, t.SortName = sh.Name, library.SortName(sh.Name)
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
	t.Aliases = library.MergeAliases(t.Aliases, sh.Name, sh.OtherNames)
	switch sh.Status {
	case "ended", "announced", "running":
		t.Status = sh.Status
	default:
		t.Status = "running"
	}
	score, votes := t.Score, t.Votes
	library.ApplyDetails(t, sh.Backdrop, sh.Logo, sh.Genres, sh.Rating, sh.People, sh.Runtime, sh.Score, sh.Votes)
	// Only TMDB gives a score, so a show on another source keeps the one it had.
	if sh.Source != "tmdb" && score != nil {
		t.Score, t.Votes = score, votes
	}
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
func seasonKeep(t mediafiles.Title, number, latest int, choice string, keepSpecials bool) bool {
	if !t.KeepUpToDate {
		return false
	}
	if number == 0 {
		return keepSpecials && (choice == library.SeasonsAll || choice == library.SeasonsFuture || (choice == "" && t.NewPartsPolicy == "all"))
	}
	switch choice {
	case library.SeasonsAll, library.SeasonsFuture:
		return true
	case library.SeasonsLatest:
		return number == latest
	case library.SeasonsNone:
		return false
	}
	return t.NewPartsPolicy == "all"
}

// futureCut is the moment before which an episode is not wanted: now when
// a show is added with Future seasons only, else its stored future_from.
func futureCut(t mediafiles.Title, choice string, now time.Time) *time.Time {
	if choice == library.SeasonsFuture {
		return &now
	}
	if choice == "" {
		return t.FutureFrom
	}
	return nil
}

// syncShowTx brings a show's seasons, episodes and Slots in line with sh.
// choice is the season choice when the show is being added, "" on a
// refresh. Episodes are matched by the source's own episode id first, then
// by (season, episode), so rows, Slots and Files survive a refresh, a
// renumbered special and a change of details source.
func syncShowTx(ctx context.Context, tx *sql.Tx, t mediafiles.Title, sh metadata.Show, defs library.ShowDefaults, choice string, now time.Time, by domain.Actor) (ShowSync, error) {
	var out ShowSync
	parts, err := database.Select[mediafiles.Part](ctx, tx, "parts", "title_id = ? AND kind = 'season'", t.ID)
	if err != nil {
		return out, err
	}
	partBy := map[int]*mediafiles.Part{}
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
	ensurePart := func(n int) (*mediafiles.Part, error) {
		if p, ok := partBy[n]; ok {
			changed := p.DroppedAt != nil
			p.DroppedAt = nil
			if nm := seasonName[n]; nm != "" && nm != p.Name {
				p.Name, changed = nm, true
			}
			if changed {
				if err := database.Update(tx, "parts", p, by); err != nil {
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
		p := &mediafiles.Part{TitleID: t.ID, Kind: "season", Number: &num, Name: name, KeepUpToDate: seasonKeep(t, n, latest, choice, defs.KeepSpecials),
			SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}}
		if _, err := database.Insert(tx, "parts", p, by); err != nil {
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
	items, err := database.Select[mediafiles.Item](ctx, tx, "items", "title_id = ? AND kind = 'episode'", t.ID)
	if err != nil {
		return out, err
	}
	itemBy := map[[2]int]*mediafiles.Item{}
	for i := range items {
		n := items[i].Numbers
		if n.Season != nil && n.Episode != nil {
			itemBy[[2]int{*n.Season, *n.Episode}] = &items[i]
		}
	}
	versions, err := database.Select[mediafiles.Version](ctx, tx, "versions", "title_id = ?", t.ID)
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
	idBy := map[string]*mediafiles.Item{}
	for i := range items {
		if v := items[i].SourceIDs[sh.Source]; v != "" {
			idBy[v] = &items[i]
		}
	}
	match := map[[2]int]*mediafiles.Item{}
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
		if err := tx.QueryRow("SELECT count(*) FROM slots WHERE item_id = ? AND ("+library.HoldingSlots+")", it.ID).Scan(&held); err != nil {
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
			if err := database.Update(tx, "items", it, by); err != nil {
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
			if err := database.Update(tx, "items", it, by); err != nil {
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
		// How release names number the episode, when that differs.
		var altS, altE *int
		if e.AltSeason > 0 && e.AltEpisode > 0 {
			as, ae := e.AltSeason, e.AltEpisode
			altS, altE = &as, &ae
		}
		if it := match[key]; it != nil {
			if sh.AltStale {
				altS, altE = it.Numbers.AltSeason, it.Numbers.AltEpisode
			}
			ids := map[string]string{}
			for k, v := range it.SourceIDs {
				ids[k] = v
			}
			if e.ID != "" {
				ids[sh.Source] = e.ID
			}
			changed := !library.EqStr(it.Name, name) || !library.EqTime(it.AirDate, e.AirDate) || it.AirTimed != (e.Timed && e.AirDate != nil) || !library.EqStr(it.AirDay, day) || !library.EqInt(it.Runtime, runtime) ||
				!library.EqInt(it.Numbers.Absolute, absolute) || !library.EqInt(it.Numbers.AltSeason, altS) || !library.EqInt(it.Numbers.AltEpisode, altE) || it.Still != e.Still || it.DroppedAt != nil || len(ids) != len(it.SourceIDs) || ids[sh.Source] != it.SourceIDs[sh.Source] ||
				it.PartID == nil || *it.PartID != p.ID || *it.Numbers.Season != e.Season || *it.Numbers.Episode != e.Number
			if !changed {
				continue
			}
			season, number := e.Season, e.Number
			it.Numbers.Season, it.Numbers.Episode = &season, &number
			it.Name, it.AirDate, it.AirDay, it.Runtime, it.Numbers.Absolute, it.DroppedAt, it.SourceIDs, it.PartID = name, e.AirDate, day, runtime, absolute, nil, ids, &p.ID
			it.Numbers.AltSeason, it.Numbers.AltEpisode = altS, altE
			it.Still, it.AirTimed = e.Still, e.Timed && e.AirDate != nil
			if err := database.Update(tx, "items", it, by); err != nil {
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
		it := &mediafiles.Item{TitleID: t.ID, PartID: &p.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: &season, Episode: &number, Absolute: absolute, AltSeason: altS, AltEpisode: altE},
			Name: name, AirDate: e.AirDate, AirDay: day, AirTimed: e.Timed && e.AirDate != nil, Runtime: runtime, KeepUpToDate: keep, SourceIDs: map[string]string{}, Still: e.Still}
		if e.ID != "" {
			it.SourceIDs[sh.Source] = e.ID
		}
		if _, err := database.Insert(tx, "items", it, by); err != nil {
			return out, err
		}
		out.EpisodesAdded++
		open, reason := library.EpisodeGate(e.AirDate, wait, now)
		state := mediafiles.SlotNotOutYet
		if open {
			state = mediafiles.SlotWanted
		}
		for _, v := range versions {
			r := reason
			if _, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: state, LastReason: &r}, by); err != nil {
				return out, err
			}
		}
	}
	if choice == library.SeasonsFuture {
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
				if err := database.Update(tx, "parts", p, by); err != nil {
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
			if err := database.Update(tx, "parts", p, by); err != nil {
				return out, err
			}
		}
	}
	out.Opened, out.Closed, err = episodeGatesTx(ctx, tx, t.ID, wait, now, by)
	return out, err
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
			if t, err := time.Parse(database.TimeLayout, air.String); err == nil {
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
		open, reason := library.EpisodeGate(r.air, wait, now)
		switch {
		case open && r.state == string(mediafiles.SlotNotOutYet):
			if err := mediafiles.MoveSlotTx(tx, r.id, mediafiles.SlotWanted, reason, by); err != nil {
				return nil, nil, err
			}
			opened = append(opened, r.id)
		case !open && r.state == string(mediafiles.SlotWanted):
			if err := mediafiles.MoveSlotTx(tx, r.id, mediafiles.SlotNotOutYet, reason, by); err != nil {
				return nil, nil, err
			}
			closed = append(closed, r.id)
		case r.state == string(mediafiles.SlotNotOutYet) && r.reason != reason:
			if _, err := tx.Exec("UPDATE slots SET last_reason = ? WHERE id = ?", reason, r.id); err != nil {
				return nil, nil, err
			}
		}
	}
	return opened, closed, nil
}
