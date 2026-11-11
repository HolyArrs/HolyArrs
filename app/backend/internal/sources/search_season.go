package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// showQuery is the search for one episode (daily: its air day; otherwise
// the season and episode release names use) or, with ep nil, one season.
func showQuery(ti mediafiles.Title, season int, ep *epSlot) Query {
	q := Query{Title: ti.Name, TV: true, Season: season}
	q.TVDB, _ = strconv.Atoi(ti.SourceIDs["tvdb"])
	q.TVMaze, _ = strconv.Atoi(ti.SourceIDs["tvmaze"])
	order := "aired"
	if ti.EpisodeOrder != nil {
		order = *ti.EpisodeOrder
	}
	switch {
	case ep == nil:
		q.Words = fmt.Sprintf("S%02d", season)
	case order == "daily" && ep.ref.AirDay != "":
		d, _ := time.Parse("2006-01-02", ep.ref.AirDay)
		q.Season, q.Episode = d.Year(), d.Format("01/02")
		q.Words = d.Format("2006 01 02")
	default:
		season, number := ep.ref.Named()
		q.Season, q.Episode = season, strconv.Itoa(number)
		q.Words = fmt.Sprintf("S%02dE%02d", season, number)
	}
	return q
}

// absoluteQuery is the second search for an episode in absolute order: the
// title and running number, as anime Releases are named ("Mashle 17"). It
// takes the shortest plain-letter name of the show.
func absoluteQuery(ti mediafiles.Title, ep *epSlot) (Query, bool) {
	if ti.EpisodeOrder == nil || *ti.EpisodeOrder != "absolute" || ep == nil || ep.ref.Absolute <= 0 {
		return Query{}, false
	}
	name := ""
	for _, a := range append([]string{ti.Name}, ti.Aliases...) {
		letters := 0
		for _, r := range a {
			if r > unicode.MaxASCII {
				letters = -1
				break
			}
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if letters >= 4 && (name == "" || len(a) < len(name)) {
			name = a
		}
	}
	if name == "" {
		name = ti.Name
	}
	return Query{TV: true, Title: fmt.Sprintf("%s %02d", name, ep.ref.Absolute)}, true
}

// handlesTV says whether a Source is set up for shows; no kinds means every kind.
func handlesTV(src domain.Source) bool {
	return len(src.Kinds) == 0 || slices.Contains(src.Kinds, "tv")
}

// tvCategories are the categories a Source uses for shows.
func tvCategories(src domain.Source) []int {
	if c := src.Categories["tv"]; len(c) > 0 {
		return c
	}
	return DefaultCategories["tv"]
}

// HandleSeason searches one season of a show for one Version.
//
// When every kept episode of the season has aired and is missing, the season
// is searched as a whole so a season pack can fill every Slot with one
// Download, and episodes nothing covered are then searched one by one.
// Otherwise each looking episode gets its own search.
//
// Params: version_id (required), by_hand, choose (false only judges and
// stores, for a person to choose from), skip_gate, bulk.
func (s *Searcher) HandleSeason(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "part" {
		return errors.New("search_season needs a part subject")
	}
	db := s.Store.Read()
	by := jobActor(r)
	jobID := r.Job.ID
	var part mediafiles.Part
	if err := mediafiles.LoadPart(ctx, db, r.Job.Subject.ID, &part); err != nil {
		return fmt.Errorf("season %d: %w", r.Job.Subject.ID, err)
	}
	var ti mediafiles.Title
	if err := mediafiles.LoadTitle(ctx, db, part.TitleID, &ti); err != nil {
		return err
	}
	vid, _ := r.Job.Params["version_id"].(float64)
	var ver mediafiles.Version
	if err := mediafiles.LoadVersion(ctx, db, int64(vid), &ver); err != nil || ver.TitleID != ti.ID {
		return fmt.Errorf("search_season needs a version_id of the show")
	}
	hand, _ := r.Job.Params["by_hand"].(bool)
	skipGate, _ := r.Job.Params["skip_gate"].(bool)
	// kept_only means a person searched the whole show, so only kept episodes
	// count. A season or episode searched by hand counts them all.
	keptOnly, _ := r.Job.Params["kept_only"].(bool)
	anyEp := hand && !keptOnly
	show := false
	if v, ok := r.Job.Params["choose"].(bool); ok && !v {
		show = true
	}
	season := 0
	if part.Number != nil {
		season = *part.Number
	}
	set, err := s.loadShow(ctx, ti, ver)
	if err != nil {
		return err
	}
	var looking, kept []*epSlot
	whole := true
	for i := range set.eps {
		e := &set.eps[i]
		if e.ref.Season != season || e.it.PartID != nil && *e.it.PartID != part.ID {
			continue
		}
		if e.kept || anyEp {
			kept = append(kept, e)
		}
		if e.sl == nil {
			continue
		}
		ok := show || slices.Contains([]mediafiles.SlotState{mediafiles.SlotWanted, mediafiles.SlotBetterWanted, mediafiles.SlotFailed, mediafiles.SlotRejected, mediafiles.SlotStalled}, e.sl.State)
		if ok && (e.kept || anyEp || show) {
			looking = append(looking, e)
		}
		missing := e.sl.State == mediafiles.SlotWanted || e.cur == nil && (e.sl.State == mediafiles.SlotFailed || e.sl.State == mediafiles.SlotRejected)
		if e.kept && !e.placeholder && (!missing || !e.soon) {
			whole = false
		}
	}
	result := map[string]any{"season": season, "version_id": ver.ID}
	if len(looking) == 0 {
		result["skipped"] = fmt.Sprintf("nothing in season %d is looking for a copy", season)
		r.SetResult(result)
		return nil
	}
	if !show && (!whole || len(looking) < 2) {
		n, err := s.queueEpisodes(ctx, looking, hand, skipGate, by)
		if err != nil {
			return err
		}
		result["episodes_queued"] = n
		r.SetResult(result)
		return nil
	}
	// rest puts the Slots still searching back to looking. It also runs when the
	// search is cut off by a stop or an error, so none is left searching; a stop
	// has ended ctx, so it writes without it.
	rest := func(reason string) {
		if show {
			return
		}
		ctx := context.WithoutCancel(ctx)
		for _, e := range looking {
			if !set.target[e.sl.ID] {
				continue
			}
			to := mediafiles.SlotWanted
			if e.cur != nil || e.sl.State == mediafiles.SlotBetterWanted {
				to = mediafiles.SlotBetterWanted
			}
			if st := s.slotState(ctx, e.sl.ID); st == mediafiles.SlotSearching {
				_ = mediafiles.MoveSlot(ctx, s.Store, e.sl.ID, to, reason, by)
				s.publish("slot", map[string]any{"id": e.sl.ID, "state": to, "reason": reason})
			}
		}
	}
	defer rest("the season search stopped before it finished")
	targets := []int64{}
	for _, e := range looking {
		set.target[e.sl.ID] = true
		targets = append(targets, e.sl.ID)
		if !show {
			if err := mediafiles.MoveSlot(ctx, s.Store, e.sl.ID, mediafiles.SlotSearching, "searching season "+strconv.Itoa(season), by); err != nil {
				delete(set.target, e.sl.ID)
				continue
			}
			s.publish("slot", map[string]any{"id": e.sl.ID, "state": mediafiles.SlotSearching})
		}
	}

	found, failures, answered, skipped, err := s.askAll(ctx, r, showQuery(ti, season, nil), hand, true, ti.Tags)
	if err != nil {
		return err
	}
	// Release names may number some episodes in other seasons (an anime
	// part TMDB keeps inside this one): ask for those seasons too.
	asked := map[int]bool{season: true}
	for _, e := range looking {
		named, _ := e.ref.Named()
		if asked[named] {
			continue
		}
		asked[named] = true
		more, _, n, _, err := s.askAll(ctx, r, showQuery(ti, named, nil), hand, true, ti.Tags)
		if err != nil {
			return err
		}
		found, answered = append(found, more...), max(answered, n)
	}
	if reason, data, ok, failed := noAnswer(failures, answered, skipped); ok {
		rest(reason)
		s.addEvent(ctx, by, &jobID, ti.ID, targets[0], EventSearchFailed, reason, data)
		if !failed {
			return nil
		}
		return errors.New(reason)
	}
	all := dedupe(found)
	rels, vs, err := s.judgeShow(ctx, set, all, hand, skipGate, s.showRules(ctx))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var picks []showPick
	if !show {
		picks = s.pickShow(ctx, all, vs, now)
	}
	foundBy := "auto_search"
	if hand {
		foundBy = "hand_search"
	}
	ids, err := s.storeAll(ctx, by, &jobID, foundBy, targets[0], all, rels, vs, show)
	if err != nil {
		return err
	}
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, e := range looking {
			if err := mediafiles.TouchItemSearchedTx(tx, e.it.ID, now); err != nil {
				return err
			}
		}
		return nil
	})
	refused := 0
	for _, v := range vs {
		if v.Outcome == "refused" {
			refused++
		}
	}
	result["releases"], result["refused"], result["sources_answered"] = len(all), refused, answered
	s.publish("releases", map[string]any{"slot_id": targets[0], "count": len(all)})
	if show {
		result["shown"] = true
		r.SetResult(result)
		return nil
	}
	chosen, held, took := s.settlePicks(ctx, by, &jobID, set.ti.ID, set.labels(), all, vs, ids, picks, "Season search")
	if chosen > 0 {
		for _, pk := range picks {
			if took[pk.slots[0]] && !pk.held {
				result["chosen_release_id"] = ids[pk.idx]
				break
			}
		}
	} else if held > 0 {
		for _, pk := range picks {
			if took[pk.slots[0]] && pk.held {
				result["held_release_id"] = ids[pk.idx]
				break
			}
		}
	}
	result["chosen"], result["held"] = chosen, held
	var left []*epSlot
	for _, e := range looking {
		if set.target[e.sl.ID] && !took[e.sl.ID] {
			left = append(left, e)
		}
	}
	if len(left) > 0 {
		reason := fmt.Sprintf("the season search found nothing for it (%d found, %d refused); searching the episode on its own", len(all), refused)
		rest(reason)
		n, err := s.queueEpisodes(ctx, left, hand, skipGate, by)
		if err != nil {
			return err
		}
		result["episodes_queued"] = n
	}
	if chosen+held == 0 {
		s.addEvent(ctx, by, &jobID, ti.ID, targets[0], EventSearchDone, fmt.Sprintf("Season %d: no Release passed the Pick rules (%d found, %d refused)", season, len(all), refused), nil)
	}
	r.SetResult(result)
	return nil
}

func (s *Searcher) slotState(ctx context.Context, id int64) mediafiles.SlotState {
	st, _ := mediafiles.SlotStateOf(ctx, s.Store.Read(), id)
	return mediafiles.SlotState(st)
}

// queueEpisodes queues an episode search for each Slot.
func (s *Searcher) queueEpisodes(ctx context.Context, eps []*epSlot, hand, skipGate bool, by domain.Actor) (int, error) {
	if s.Queue == nil {
		return 0, errors.New("searching episodes needs the job queue")
	}
	n := 0
	start := time.Now().UTC()
	for _, e := range eps {
		params := map[string]any{}
		key := "search_slot:" + strconv.FormatInt(e.sl.ID, 10)
		if hand {
			params["by_hand"] = true
			key = "hand_search_slot:" + strconv.FormatInt(e.sl.ID, 10)
		}
		if skipGate {
			params["skip_gate"] = true
		}
		_, created, err := s.Queue.EnqueueAt(ctx, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: e.sl.ID}, params, key, by, start.Add(time.Duration(n)*bulkStagger))
		if err != nil {
			return n, err
		}
		if created {
			n++
		}
	}
	return n, nil
}
