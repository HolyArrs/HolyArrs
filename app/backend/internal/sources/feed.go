package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"

	"github.com/HolyArrs/holyarrs/app/backend/internal/judge"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/tagset"
)

const (
	// Settings in the what_to_pick group that steer the searches.
	SettingMissingCount = "missing_search_count"
	// SettingMissingBetter makes the scheduled search look for better copies too.
	SettingMissingBetter = "missing_search_better"
	// SettingRespectsGate makes bulk searches skip Slots that are not out yet.
	SettingRespectsGate = "bulk_search_respects_gate"
	// SettingBulkCap is the most Slots one bulk search asked by a person queues.
	SettingBulkCap = "bulk_search_cap"
	// defaultBulkCap is used when SettingBulkCap is unset or not a positive number.
	defaultBulkCap = 500
	// bulkStagger is the private name for BulkStagger.
	bulkStagger = BulkStagger

	// BulkStagger is the gap between the searches one bulk search queues, so Sources are not asked all at once.
	BulkStagger = 2 * time.Second

	// SettingArtistSearchCap is the most album searches one artist search queues. The rest wait for the next request or the nightly search.
	SettingArtistSearchCap = "artist_search_cap"

	// Releases posted well before their movie, episode or album came out, or far too small for what they hold, are refused. Both checks are on unless set to "false".
	SettingRefuseEarly = "refuse_early_releases"
	SettingEarlyGrace  = "early_release_grace_hours"
	SettingRefuseTiny  = "refuse_tiny_releases"

	// SettingHoldUnknown holds a Release of unknown size or date for a person to choose in automatic searches. Off by default, so such a Release only gets a warning.
	SettingHoldUnknown = "hold_unknown_releases"

	// SettingEpisodeMinutes is the length used for an episode of unknown length by the fake-size floor.
	SettingEpisodeMinutes = "episode_default_minutes"

	// SettingImportFloors turns the fixed least sizes at import off when set to "false".
	SettingImportFloors = "import_size_floors"
	// Defaults and limits for the settings above.
	DefaultEpisodeMinutes  = 20
	DefaultEarlyGrace      = 24
	DefaultArtistSearchCap = 200
	defaultMissingCount    = 20
	maxMissingCount        = 500
	searchSettingsGroup    = "what_to_pick"
	// searchMissingByPerson marks a missing search started by a person.
	searchMissingByPerson = "by_person"
)

// lookingStates are the Slot states a feed check picks for: missing, a better copy wanted, or holding a Release that something better may beat.
var lookingStates = []domain.SlotState{domain.SlotWanted, domain.SlotBetterWanted, domain.SlotHeld}

// askCatchUpPerRun is the most approved-ask titles one missing search catches up, since each is searched in full. A variable so tests can change it.
var askCatchUpPerRun = 3

// Register adds the search Job handlers to a queue.
func (s *Searcher) Register(q *jobs.Queue) {
	q.Handle(jobs.KindSearchSlot, s.Handle)
	q.Handle(jobs.KindFeedCheck, s.HandleFeed)
	q.Handle(jobs.KindGrabHeld, s.HandleGrabHeld)
	q.Handle(jobs.KindSearchMissing, s.HandleSearchMissing)
	q.Handle(jobs.KindSearchSeason, s.HandleSeason)
	q.Handle(jobs.KindSearchAlbum, s.HandleAlbum)
}

// HandleFeed reads the newest Releases of every feed Source, matches them to Titles whose Slots want one, judges them per Slot, and chooses or holds the best.
// A feed Release is looked at once: a name already stored by the feed check, or still waiting as judged, is skipped. Names stored and settled by other searches are looked at again.
// Held Releases whose wait is over are chosen at the end.
func (s *Searcher) HandleFeed(ctx context.Context, r *jobs.Run) error {
	by := jobActor(r)
	jobID := r.Job.ID
	now := time.Now().UTC()
	db := s.Store.Read()
	srcs, err := database.Select[domain.Source](ctx, db, "sources",
		"(state = 'enabled' OR (state = 'paused' AND paused_until <= ?)) ORDER BY priority, id", database.FormatTime(now))
	if err != nil {
		return err
	}
	var all []found
	var failures []string
	read := 0
	var showsWanted, musicWanted bool
	_ = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE t.kind = 'artist' AND t.state = 'active' AND s.state IN ('wanted', 'better_wanted', 'held'))`).Scan(&musicWanted)
	_ = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE t.kind = 'show' AND t.state = 'active' AND s.state IN ('wanted', 'better_wanted', 'held'))`).Scan(&showsWanted)
	prepared, _ := r.TakePrepared().(*feedPrepared)
	if prepared != nil && (prepared.fingerprint != feedSourceFingerprint(srcs) || prepared.showsWanted != showsWanted || prepared.musicWanted != musicWanted) {
		prepared = nil
	}
	if prepared != nil {
		for _, answer := range prepared.answers {
			src := answer.src
			if !src.UseFor.Feed {
				continue
			}
			if answer.observed {
				Record(context.Background(), s.Store, src.ID, "feed", answer.callTook, answer.callErr)
			}
			if answer.err != nil {
				failures = append(failures, src.Name+": "+answer.err.Error())
				s.pause(ctx, src, answer.err)
				continue
			}
			read++
			s.recovered(ctx, src)
			for _, it := range answer.items {
				var seen int
				if err := db.QueryRowContext(ctx, "SELECT count(*) FROM releases WHERE source_id = ? AND name = ? AND (found_by = 'feed' OR state = 'judged')", src.ID, it.Title).Scan(&seen); err != nil {
					return err
				}
				if seen == 0 {
					all = append(all, found{src: src, item: it, keys: answer.keys})
				}
			}
		}
	} else {
		for i, src := range srcs {
			if !src.UseFor.Feed {
				continue
			}
			_ = r.Progress(ctx, float64(i)/float64(len(srcs)+1), "reading "+src.Name)
			if s.onAsk != nil {
				s.onAsk(src.Name)
			}
			cl, err := s.client(ctx, src)
			var items []Item
			if err == nil {
				cats := src.Categories["movie"]
				if handlesTV(src) && showsWanted {

					// One feed read serves movies and shows.
					cats = append(slices.Clone(cats), tvCategories(src)...)
				}
				if handlesMusic(src) && musicWanted {

					// The same read serves albums too.
					cats = append(slices.Clone(cats), musicCategories(src)...)
				}
				items, err = cl.Feed(ctx, cats)
			}
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				failures = append(failures, src.Name+": "+err.Error())
				s.pause(ctx, src, err)
				continue
			}
			read++
			s.recovered(ctx, src)
			for _, it := range items {
				var seen int
				if err := db.QueryRowContext(ctx, "SELECT count(*) FROM releases WHERE source_id = ? AND name = ? AND (found_by = 'feed' OR state = 'judged')", src.ID, it.Title).Scan(&seen); err != nil {
					return err
				}
				if seen == 0 {
					all = append(all, found{src: src, item: it, keys: secretValues(cl.cfg)})
				}
			}
		}
	}
	all = dedupe(all)
	// Nothing to match: skip loading Slots, which at library scale costs one record per episode or track.
	if len(all) == 0 {

		grabbed, err := s.grabHeld(ctx, by, &jobID, now)
		if err != nil {
			return err
		}
		result := map[string]any{"sources_read": read, "releases_new": 0, "releases_stored": 0, "chosen": 0, "held": 0, "held_chosen": grabbed}
		if len(failures) > 0 {
			result["failures"] = failures
		}
		r.SetResult(result)
		if read > 0 || len(failures) > 0 {
			summary := fmt.Sprintf("Feed check: %d Sources read, 0 new Releases, %d chosen, 0 held", read, grabbed)
			s.addEvent(ctx, by, &jobID, 0, 0, EventFeedChecked, summary, result)
		}
		if read == 0 && len(failures) > 0 {
			return fmt.Errorf("no feed Source answered: %s", strings.Join(failures, "; "))
		}
		return nil
	}

	// Match against one small record per Title first. A feed normally names only a few Titles, so loading every Slot would waste memory and queries.
	refs, err := s.wantedTitleRefs(ctx)
	if err != nil {
		return err
	}
	hasShow := slices.ContainsFunc(refs, func(r judge.TitleRef) bool { return r.Kind == "show" })
	hasArtist := slices.ContainsFunc(refs, func(r judge.TitleRef) bool { return r.Kind == "artist" })
	byTitle := map[int64][]found{}
	for _, f := range all {
		m := judge.MatchTitle(releasename.Parse(f.item.Title, releasename.Hints{Size: f.item.Size}), refs)
		if m.Title == nil && hasShow {

			// Read again as a show name ("Part 2" before tags, "- 12").
			m = judge.MatchTitle(releasename.Parse(f.item.Title, releasename.Hints{Size: f.item.Size, Show: true}), refs)
		}
		if m.Title == nil && hasArtist {

			// Read again as an album name ("Artist - Album (Year) [FLAC]").
			m = judge.MatchTitle(releasename.Parse(f.item.Title, releasename.Hints{Size: f.item.Size, Music: true}), refs)
		}

		// A tagged Source's Release goes only to a Title sharing a tag. It is matched against every wanted Title first, so it never lands on a look-alike because the right one was left out.
		if m.Title != nil && tagset.SourceServes(f.src.Tags, m.Title.Tags) {
			byTitle[m.Title.ID] = append(byTitle[m.Title.ID], f)
		}
	}
	ctxs := make(map[int64][]slotCtx, len(byTitle))
	for titleID := range byTitle {
		slots, err := database.Select[domain.Slot](ctx, db, "slots", `state IN (?, ?, ?) AND item_id IN (
			SELECT id FROM items WHERE title_id = ?) ORDER BY id`,
			string(lookingStates[0]), string(lookingStates[1]), string(lookingStates[2]), titleID)
		if err != nil {
			return err
		}
		for _, sl := range slots {
			c, err := s.loadSlot(ctx, sl.ID)
			if err != nil {
				return err
			}
			ctxs[titleID] = append(ctxs[titleID], c)
		}
	}

	chosen, held, stored := 0, 0, 0
	// Each new Release is stored once; later Slots update it.
	storedID := map[string]int64{}
	keyOf := func(f found) string { return strconv.FormatInt(f.src.ID, 10) + "|" + f.item.Title }
	for titleID, fs := range byTitle {
		if cs := ctxs[titleID]; len(cs) > 0 && cs[0].ti.Kind == "show" {
			c1, h1, n, err := s.feedShow(ctx, by, &jobID, cs, fs, storedID, keyOf)
			if err != nil {
				return err
			}
			chosen, held, stored = chosen+c1, held+h1, stored+n
			continue
		}
		if cs := ctxs[titleID]; len(cs) > 0 && cs[0].ti.Kind == "artist" {
			c1, h1, n, err := s.feedArtist(ctx, by, &jobID, cs, fs, storedID, keyOf)
			if err != nil {
				return err
			}
			chosen, held, stored = chosen+c1, held+h1, stored+n
			continue
		}
		for _, c := range ctxs[titleID] {
			cands := append([]found(nil), fs...)
			for k := range cands {
				if id := storedID[keyOf(cands[k])]; id > 0 {
					cands[k].relID = id
				}
			}
			if c.sl.State == domain.SlotHeld && c.sl.HeldReleaseID != nil {
				if f, ok := s.storedFound(ctx, *c.sl.HeldReleaseID); ok {
					cands = append(cands, f)
				}
			}
			rels, vs, err := s.judgeAll(ctx, c, cands, true, false, false)
			if err != nil {
				return err
			}
			w := s.pickWinner(ctx, cands, vs, now)
			ids, err := s.storeAll(ctx, by, &jobID, "feed", c.sl.ID, cands, rels, vs, false)
			if err != nil {
				return err
			}
			for k := range cands {
				if cands[k].relID == 0 {
					storedID[keyOf(cands[k])] = ids[k]
					stored++
				}
			}
			if beforeSettle != nil {
				beforeSettle(c.sl.ID)
			}
			switch {
			case w >= 0 && vs[w].Outcome == "chosen":
				name := cands[w].item.Title
				if err := s.settleChosen(ctx, c.sl.ID, ids[w], name, by); errors.Is(err, ErrBusy) {
					continue
				} else if err != nil {
					return err
				}
				chosen++
				s.addEvent(ctx, by, &jobID, c.ti.ID, c.sl.ID, EventReleaseChosen, "Feed check chose "+name, map[string]any{"release_id": ids[w]})
				s.publish("slot", map[string]any{"id": c.sl.ID, "state": domain.SlotChosen, "reason": "chose " + name})
				if s.OnChosen != nil {
					s.OnChosen(ctx, c.sl.ID, ids[w])
				}
			case w >= 0 && vs[w].Outcome == "held" && (c.sl.HeldReleaseID == nil || *c.sl.HeldReleaseID != ids[w]):
				name := cands[w].item.Title
				if err := s.settleHeld(ctx, c.sl.ID, ids[w], name, *vs[w].HoldUntil, by); errors.Is(err, ErrBusy) {
					continue
				} else if err != nil {
					return err
				}
				held++
				s.addEvent(ctx, by, &jobID, c.ti.ID, c.sl.ID, EventReleaseHeld, "Feed check holding "+name, map[string]any{"release_id": ids[w]})
				s.publish("slot", map[string]any{"id": c.sl.ID, "state": domain.SlotHeld, "reason": "holding " + name})
			}
		}
	}
	grabbed, err := s.grabHeld(ctx, by, &jobID, now)
	if err != nil {
		return err
	}
	result := map[string]any{"sources_read": read, "releases_new": len(all), "releases_stored": stored, "chosen": chosen, "held": held, "held_chosen": grabbed}
	if len(failures) > 0 {
		result["failures"] = failures
	}
	r.SetResult(result)
	if read > 0 || len(failures) > 0 {
		summary := fmt.Sprintf("Feed check: %d Sources read, %d new Releases, %d chosen, %d held", read, len(all), chosen+grabbed, held)
		s.addEvent(ctx, by, &jobID, 0, 0, EventFeedChecked, summary, result)
	}
	if read == 0 && len(failures) > 0 {
		return fmt.Errorf("no feed Source answered: %s", strings.Join(failures, "; "))
	}
	return nil
}

// ProbeGrabHeld reports whether the scheduled held-Release Job can change a Slot. It only runs the due check that grabHeld uses.
func (s *Searcher) ProbeGrabHeld(ctx context.Context, now time.Time) (bool, any, error) {
	var due bool
	err := s.Store.Read().QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM slots WHERE state = 'held' AND held_release_id IS NOT NULL AND next_try_at <= ?)`,
		database.FormatTime(now.UTC())).Scan(&due)
	return due, nil, err
}

// HandleGrabHeld chooses every Held Release whose wait is over.
func (s *Searcher) HandleGrabHeld(ctx context.Context, r *jobs.Run) error {
	jobID := r.Job.ID
	n, err := s.grabHeld(ctx, jobActor(r), &jobID, time.Now().UTC())
	if err != nil {
		return err
	}
	r.SetResult(map[string]any{"chosen": n})
	return nil
}

// HandleSearchMissing queues a search for Slots still looking, least recently searched first.
// The scheduled run takes up to missing_search_count wanted Slots, plus better_wanted ones when missing_search_better is on. A person can pass which ("missing" or "better") to search the whole list up to bulk_search_cap. Slots not out yet are skipped while bulk_search_respects_gate is on.
func (s *Searcher) HandleSearchMissing(ctx context.Context, r *jobs.Run) error {
	db := s.Store.Read()
	if s.Queue == nil {
		return fmt.Errorf("search_missing needs the job queue")
	}
	states := []string{string(domain.SlotWanted)}
	limit := defaultMissingCount
	if n, err := strconv.Atoi(Setting(ctx, db, SettingMissingCount)); err == nil && n >= 0 {
		limit = min(n, maxMissingCount)
	}
	if Setting(ctx, db, SettingMissingBetter) == "true" {
		states = append(states, string(domain.SlotBetterWanted))
	}
	if w, _ := r.Job.Params["which"].(string); w != "" {

		limit = defaultBulkCap
		if n, err := strconv.Atoi(Setting(ctx, db, SettingBulkCap)); err == nil && n > 0 {
			limit = n
		}
		switch w {
		case "missing":
			states = []string{string(domain.SlotWanted)}
		case "better":
			states = []string{string(domain.SlotBetterWanted)}
		default:
			return fmt.Errorf("which must be missing or better")
		}
	}

	// title_id keeps the search to one Title and all it still wants, as an approved request for a show or artist asks.
	titleID, _ := r.Job.Params["title_id"].(float64)
	if titleID > 0 {
		limit = maxMissingCount
	}
	skipGate := Setting(ctx, db, SettingRespectsGate) == "false"
	if skipGate && states[0] == string(domain.SlotWanted) {
		states = append(states, string(domain.SlotNotOutYet))
	}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(states)), ", ")

	// A person may keep a bulk search to one kind (movie, show or artist).
	only, _ := r.Job.Params["kind"].(string)

	// The shows and music switches apply to nightly and bulk searches. A search kept to one Title always covers its shows and music.
	argsFor := func(shows, music bool) []any {
		a := []any{}
		for _, st := range states {
			a = append(a, st)
		}
		return append(a, only, only, int64(titleID), int64(titleID), shows, music)
	}
	args := argsFor(titleID > 0 || Setting(ctx, db, SettingMissingShows) != "false", titleID > 0 || Setting(ctx, db, SettingMissingMusic) != "false")
	from := ` FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id
		JOIN titles t ON t.id = i.title_id
		WHERE s.state IN (` + ph + `) AND (? = '' OR t.kind = ?) AND (? = 0 OR t.id = ?) AND i.keep_up_to_date = 1 AND v.keep_up_to_date = 1 AND t.state = 'active' AND s.active_download_id IS NULL
		AND (t.kind = 'movie' OR (? AND t.kind = 'show' AND t.keep_up_to_date = 1 AND s.state != 'not_out_yet' AND i.dropped_at IS NULL
			AND (i.part_id IS NULL OR EXISTS (SELECT 1 FROM parts p WHERE p.id = i.part_id AND p.keep_up_to_date = 1 AND p.dropped_at IS NULL)))
			OR (? AND t.kind = 'artist' AND t.keep_up_to_date = 1 AND s.state != 'not_out_yet' AND i.dropped_at IS NULL
			AND EXISTS (SELECT 1 FROM parts p WHERE p.id = i.part_id AND p.keep_up_to_date = 1 AND p.included = 1 AND p.dropped_at IS NULL)))`

	// Approved requests go first. A Title with an approved ask and something never searched gets the one-title search that approval starts, oldest approval first, whatever the cap. The same Job is used so the two never run twice.
	// It is skipped while approval's own scan and search are still to run, and after a one-title search was queued since the approval. Then the Title rejoins the oldest-searched-first turn below. Without this, an ask approved behind a large never-searched Library would wait for months.
	// Like approval, it covers shows and music whatever the nightly switches, unless "Search when approved" is off.
	asked := 0
	var askTitles []string
	var startAfter time.Time
	if titleID == 0 && states[0] == string(domain.SlotWanted) && limit > 0 {
		var onApprove string
		_ = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = 'ask_search_on_approve'").Scan(&onApprove)
		approveOn := onApprove != "false"

		// The approved asks are few. The jobs table only grows, so it is read once for the last one-title search of each Title, and through its state index for what is still to run.
		cq := `WITH c AS (SELECT a.title_id AS id, MIN(a.decided_at) AS d FROM asks a WHERE a.state = 'approved' AND a.title_id IS NOT NULL GROUP BY a.title_id),
			w AS (SELECT dedupe_key AS k, MAX(created_at) AS m FROM jobs WHERE dedupe_key LIKE 'search_wanted:%' GROUP BY dedupe_key)
			SELECT c.id, (SELECT count(*)` + from + ` AND t.id = c.id) FROM c LEFT JOIN w ON w.k = 'search_wanted:' || c.id
			WHERE (w.m IS NULL OR w.m < c.d) AND EXISTS (SELECT 1` + from + ` AND t.id = c.id AND i.last_searched_at IS NULL)
			AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.state IN ('queued', 'running') AND (j.dedupe_key = 'search_wanted:' || c.id
				OR (j.kind IN ('refresh_title', 'scan_title') AND json_extract(j.subject, '$.type') = 'title' AND json_extract(j.subject, '$.id') = c.id AND j.params LIKE '%search%')))
			ORDER BY c.d, c.id LIMIT ` + strconv.Itoa(min(limit, askCatchUpPerRun))
		ca := argsFor(approveOn || Setting(ctx, db, SettingMissingShows) != "false", approveOn || Setting(ctx, db, SettingMissingMusic) != "false")
		trows, err := db.QueryContext(ctx, cq, append(append([]any{}, ca...), ca...)...)
		if err != nil {
			return err
		}
		type askTitle struct{ id, slots int64 }
		var tids []askTitle
		for trows.Next() {
			var a askTitle
			if err := trows.Scan(&a.id, &a.slots); err != nil {
				trows.Close()
				return err
			}
			tids = append(tids, a)
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return err
		}

		// Each Title's searches are spread bulkStagger apart from when its Job starts. The next Title starts after the last of them (its Slots are the most it can queue), so Titles do not ask the Sources together. The per-run cap keeps this wait short.
		at := time.Now().UTC()
		for _, a := range tids {
			if _, _, err := s.Queue.EnqueueAt(ctx, jobs.KindSearchMissing, &domain.Subject{Type: "title", ID: a.id},
				map[string]any{"title_id": a.id}, "search_wanted:"+strconv.FormatInt(a.id, 10), jobActor(r), at); err != nil {
				return err
			}
			at = at.Add(time.Duration(min(a.slots, maxMissingCount)) * bulkStagger)
			asked++
			askTitles = append(askTitles, strconv.FormatInt(a.id, 10))
		}
		startAfter = at
	}
	q := `SELECT s.id, t.kind, COALESCE(i.part_id, 0), s.version_id` + from
	if len(askTitles) > 0 {

		// Their own search covers them, so the cap goes to the rest.
		q += ` AND t.id NOT IN (` + strings.Join(askTitles, ", ") + `)`
	}
	q += ` ORDER BY i.last_searched_at IS NOT NULL, i.last_searched_at, s.id`
	if limit >= 0 {
		q += " LIMIT " + strconv.Itoa(limit)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	type found struct {
		id, part, ver int64
		kind          string
	}
	var ids []int64
	var list []found
	for rows.Next() {
		var f found
		if err := rows.Scan(&f.id, &f.kind, &f.part, &f.ver); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, f.id)
		list = append(list, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Episodes of one season in one Version are searched together as a season when there are several. A single one gets an episode search.
	type seasonKey struct{ part, ver int64 }
	perSeason := map[seasonKey]int{}
	for _, f := range list {
		if f.kind == "show" && f.part > 0 {
			perSeason[seasonKey{f.part, f.ver}]++
		}
	}
	queued, seasons, albums := 0, 0, 0
	start := time.Now().UTC()
	if startAfter.After(start) {
		start = startAfter
	}
	seen := map[seasonKey]bool{}
	for _, f := range list {
		params := map[string]any{"bulk": true}
		if skipGate {
			params["skip_gate"] = true
		}
		kind, subject, key := jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.id}, "search_slot:"+strconv.FormatInt(f.id, 10)
		if k := (seasonKey{f.part, f.ver}); f.kind == "show" && perSeason[k] > 1 {
			if seen[k] {
				continue
			}
			seen[k] = true
			params["version_id"] = f.ver
			kind, subject = jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.part}
			key = fmt.Sprintf("search_season:%d:%d", f.part, f.ver)
		}
		// Tracks are always searched as their album.
		if k := (seasonKey{f.part, f.ver}); f.kind == "artist" && f.part > 0 {

			if seen[k] {
				continue
			}
			seen[k] = true
			params["version_id"] = f.ver
			kind, subject = jobs.KindSearchAlbum, &domain.Subject{Type: "part", ID: f.part}
			key = fmt.Sprintf("search_album:%d:%d", f.part, f.ver)
		}

		// Spread the searches so Sources are not asked all at once. The time goes in with the Job, so no worker starts it early.
		if _, created, err := s.Queue.EnqueueAt(ctx, kind, subject, params, key, jobActor(r), start.Add(time.Duration(queued)*bulkStagger)); err != nil {
			return err
		} else if created {
			queued++
			if kind == jobs.KindSearchSeason {
				seasons++
			}
			if kind == jobs.KindSearchAlbum {
				albums++
			}
		}
	}
	r.SetResult(map[string]any{"slots": len(ids), "queued": queued, "seasons": seasons, "albums": albums, "asks": asked})
	return nil
}

// Setting reads one what_to_pick setting and returns "" when unset.
func Setting(ctx context.Context, db *sql.DB, key string) string {
	var v string
	_ = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", searchSettingsGroup, key).Scan(&v)
	return v
}

// Checks reads the early-release and fake-size settings that judging needs.
func Checks(ctx context.Context, db *sql.DB) judge.Checks {
	grace := DefaultEarlyGrace
	if n, err := strconv.Atoi(Setting(ctx, db, SettingEarlyGrace)); err == nil && n >= 0 && n <= 720 {
		grace = n
	}
	minutes := DefaultEpisodeMinutes
	if n, err := strconv.Atoi(Setting(ctx, db, SettingEpisodeMinutes)); err == nil && n >= 1 && n <= 240 {
		minutes = n
	}
	return judge.Checks{Early: Setting(ctx, db, SettingRefuseEarly) != "false", Grace: time.Duration(grace) * time.Hour,
		Tiny: Setting(ctx, db, SettingRefuseTiny) != "false", HoldUnknown: Setting(ctx, db, SettingHoldUnknown) == "true",
		EpisodeMinutes: minutes}
}

// wantedTitleRefs lists one small record per active Title that still has a Slot looking, so a feed can be matched without loading every Slot.
func (s *Searcher) wantedTitleRefs(ctx context.Context) ([]judge.TitleRef, error) {
	titles, err := database.Select[domain.Title](ctx, s.Store.Read(), "titles", `state = 'active' AND EXISTS (
		SELECT 1 FROM items i JOIN slots sl ON sl.item_id = i.id
		WHERE i.title_id = titles.id AND sl.state IN (?, ?, ?)) ORDER BY sort_name, id`,
		string(lookingStates[0]), string(lookingStates[1]), string(lookingStates[2]))
	if err != nil {
		return nil, err
	}
	refs := make([]judge.TitleRef, 0, len(titles))
	for _, title := range titles {
		ref := judge.TitleRef{ID: title.ID, Name: title.Name, Aliases: title.Aliases, Tags: title.Tags}
		if title.Year != nil {
			ref.Year = *title.Year
		}
		if title.OriginalLanguage != nil {
			ref.OriginalLanguage = *title.OriginalLanguage
		}
		if title.Kind == "show" || title.Kind == "artist" {
			ref.Kind = title.Kind
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// storedFound turns a held Release back into a candidate. Its link is already stored without keys.
func (s *Searcher) storedFound(ctx context.Context, id int64) (found, bool) {
	var rel domain.Release
	if err := database.Get(ctx, s.Store.Read(), "releases", id, &rel); err != nil || rel.State != "held" {
		return found{}, false
	}
	var src domain.Source
	if err := database.Get(ctx, s.Store.Read(), "sources", rel.SourceID, &src); err != nil {
		return found{}, false
	}
	it := Item{Title: rel.Name, Link: rel.LinkRef, Size: rel.Size, Protocol: rel.Protocol, Seeders: rel.Seeders, Peers: rel.Peers}
	if rel.PublishedAt != nil {
		it.Published = *rel.PublishedAt
	}
	if rel.InfoHash != nil {
		it.InfoHash = *rel.InfoHash
	}
	return found{src: src, item: it, relID: rel.ID}, true
}

// grabHeld chooses the Held Releases whose wait is over and returns how many Slots were settled.
func (s *Searcher) grabHeld(ctx context.Context, by domain.Actor, jobID *int64, now time.Time) (int, error) {
	slots, err := database.Select[domain.Slot](ctx, s.Store.Read(), "slots",
		"state = 'held' AND held_release_id IS NOT NULL AND next_try_at <= ? ORDER BY next_try_at, id", database.FormatTime(now))
	if err != nil {
		return 0, err
	}
	n := 0
	done := map[int64]bool{}
	for _, sl := range slots {
		if done[sl.ID] {
			continue
		}
		var rel domain.Release
		if err := database.Get(ctx, s.Store.Read(), "releases", *sl.HeldReleaseID, &rel); err != nil || rel.State != "held" {
			continue
		}
		// The files turned out to be a program during the wait: refuse the Release and send its Slots back to looking.
		if s.dangerousRel(ctx, rel) {

			_ = s.Store.Write(ctx, func(tx *sql.Tx) error {

				// Only while it is still held, since a person may have chosen it meanwhile.
				res, err := tx.Exec(`UPDATE releases SET state = 'refused', hold_until = NULL,
					verdict = json_set(COALESCE(verdict, '{}'), '$.outcome', 'refused', '$.dangerous_files', json('true')), updated_at = ?, updated_by = ? WHERE id = ? AND state = 'held'`,
					database.FormatTime(now), actorText(by), rel.ID)
				if err != nil {
					return err
				}
				if n, err := res.RowsAffected(); err != nil || n == 0 {
					return err
				}
				return unholdTx(tx, rel.ID, "not sent: "+dangerousWhy, by)
			})
			done[sl.ID] = true
			continue
		}

		// A season pack held for several Slots is chosen for all that still hold it. A Slot missing from the pick list (an old list) is chosen alone.
		pick := []int64{sl.ID}
		if many := PickSlots(rel); len(many) > 1 && slices.Contains(many, sl.ID) {
			pick = pick[:0]
			for _, o := range slots {
				if o.HeldReleaseID != nil && *o.HeldReleaseID == rel.ID && slices.Contains(many, o.ID) {
					pick = append(pick, o.ID)
				}
			}
		}
		for _, id := range pick {
			done[id] = true
		}
		if err := s.settleChosenMany(ctx, pick, rel.ID, rel.Name, by); errors.Is(err, ErrBusy) {
			continue
		} else if err != nil {
			return n, err
		}
		n++
		var titleID int64
		_ = s.Store.Read().QueryRowContext(ctx, "SELECT title_id FROM items WHERE id = ?", sl.ItemID).Scan(&titleID)
		for _, id := range pick {
			s.addEvent(ctx, by, jobID, titleID, id, EventReleaseChosen, "Wait over, chose "+rel.Name, map[string]any{"release_id": rel.ID, "was_held": true})
			s.publish("slot", map[string]any{"id": id, "state": domain.SlotChosen, "reason": "chose " + rel.Name})
		}
		if s.OnChosen != nil {
			s.OnChosen(ctx, sl.ID, rel.ID)
		}
	}
	return n, nil
}

// movieOut returns a movie's first known release day: cinema, digital or disc.
func movieOut(t domain.Title) *time.Time {
	var first *time.Time
	for _, d := range []*time.Time{t.CinemaDate, t.DigitalDate, t.PhysicalDate} {
		if d != nil && (first == nil || d.Before(*first)) {
			first = d
		}
	}
	return first
}
