package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/judge"
	"github.com/CoderLuii/holyarrs/internal/parse"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Job kinds for stage 3.1; each also has a schedules row of the same kind.
const (
	KindFeedCheck     = "feed_check"     // read every feed Source, pick for Slots that want one
	KindGrabHeld      = "grab_held"      // choose Held Releases whose wait is over
	KindSearchMissing = "search_missing" // queue searches for Slots still looking
)

// Register adds the search Job handlers to a queue.
func (s *Searcher) Register(q *jobs.Queue) {
	q.Handle(KindSearchSlot, s.Handle)
	q.Handle(KindFeedCheck, s.HandleFeed)
	q.Handle(KindGrabHeld, s.HandleGrabHeld)
	q.Handle(KindSearchMissing, s.HandleSearchMissing)
	q.Handle(KindSearchSeason, s.HandleSeason)
	q.Handle(KindSearchAlbum, s.HandleAlbum)
}

// lookingStates are the Slot states a feed check picks for: still missing,
// a better copy wanted, or holding a Release that something better may beat.
var lookingStates = []domain.SlotState{domain.SlotWanted, domain.SlotBetterWanted, domain.SlotHeld}

// HandleFeed reads the newest Releases of every Source used for feeds,
// matches them to Titles whose Slots want one, judges them per Slot and
// chooses or holds the best. Each feed Release is looked at once: a name
// the feed check already stored from that Source, or one still waiting as
// judged, is skipped; names other searches stored and settled are looked
// at again. Held Releases whose wait is
// over are chosen at the end.
func (s *Searcher) HandleFeed(ctx context.Context, r *jobs.Run) error {
	by := jobActor(r)
	jobID := r.Job.ID
	now := time.Now().UTC()
	db := s.Store.Read()
	srcs, err := store.Select[domain.Source](ctx, db, "sources",
		"(state = 'enabled' OR (state = 'paused' AND paused_until <= ?)) ORDER BY priority, id", store.FormatTime(now))
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
	if len(all) == 0 {
		// Do not materialize every wanted Slot when the feeds had nothing to
		// match. At library scale that retained one slotCtx per episode or
		// track, even though the matching loop below had no candidates.
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

	// Match against one compact record per Title first. Loading every wanted
	// Slot here retained the whole library and did five point queries per Slot,
	// although a feed normally names only a few Titles.
	refs, err := s.wantedTitleRefs(ctx)
	if err != nil {
		return err
	}
	hasShow := slices.ContainsFunc(refs, func(r judge.TitleRef) bool { return r.Kind == "show" })
	hasArtist := slices.ContainsFunc(refs, func(r judge.TitleRef) bool { return r.Kind == "artist" })
	byTitle := map[int64][]found{}
	for _, f := range all {
		m := judge.MatchTitle(parse.Parse(f.item.Title, parse.Hints{Size: f.item.Size}), refs)
		if m.Title == nil && hasShow {
			// Read again as a show name ("Part 2" before tags, "- 12").
			m = judge.MatchTitle(parse.Parse(f.item.Title, parse.Hints{Size: f.item.Size, Show: true}), refs)
		}
		if m.Title == nil && hasArtist {
			// Read again as an album name ("Artist - Album (Year) [FLAC]").
			m = judge.MatchTitle(parse.Parse(f.item.Title, parse.Hints{Size: f.item.Size, Music: true}), refs)
		}
		if m.Title != nil {
			byTitle[m.Title.ID] = append(byTitle[m.Title.ID], f)
		}
	}
	ctxs := make(map[int64][]slotCtx, len(byTitle))
	for titleID := range byTitle {
		slots, err := store.Select[domain.Slot](ctx, db, "slots", `state IN (?, ?, ?) AND item_id IN (
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
	storedID := map[string]int64{} // each new Release is stored once; later Slots update it
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
					continue // another pick got this Slot first
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

func (s *Searcher) wantedTitleRefs(ctx context.Context) ([]judge.TitleRef, error) {
	titles, err := store.Select[domain.Title](ctx, s.Store.Read(), "titles", `state = 'active' AND EXISTS (
		SELECT 1 FROM items i JOIN slots sl ON sl.item_id = i.id
		WHERE i.title_id = titles.id AND sl.state IN (?, ?, ?)) ORDER BY sort_name, id`,
		string(lookingStates[0]), string(lookingStates[1]), string(lookingStates[2]))
	if err != nil {
		return nil, err
	}
	refs := make([]judge.TitleRef, 0, len(titles))
	for _, title := range titles {
		ref := judge.TitleRef{ID: title.ID, Name: title.Name, Aliases: title.Aliases}
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

// ProbeGrabHeld reports whether the scheduled held-Release Job can change a
// Slot. It reads only the due predicate used by grabHeld.
func (s *Searcher) ProbeGrabHeld(ctx context.Context, now time.Time) (bool, any, error) {
	var due bool
	err := s.Store.Read().QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM slots WHERE state = 'held' AND held_release_id IS NOT NULL AND next_try_at <= ?)`,
		store.FormatTime(now.UTC())).Scan(&due)
	return due, nil, err
}

// storedFound turns a stored Release back into a candidate (its link stays
// as stored, already without keys).
func (s *Searcher) storedFound(ctx context.Context, id int64) (found, bool) {
	var rel domain.Release
	if err := store.Get(ctx, s.Store.Read(), "releases", id, &rel); err != nil || rel.State != "held" {
		return found{}, false
	}
	var src domain.Source
	if err := store.Get(ctx, s.Store.Read(), "sources", rel.SourceID, &src); err != nil {
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

func (s *Searcher) grabHeld(ctx context.Context, by domain.Actor, jobID *int64, now time.Time) (int, error) {
	slots, err := store.Select[domain.Slot](ctx, s.Store.Read(), "slots",
		"state = 'held' AND held_release_id IS NOT NULL AND next_try_at <= ? ORDER BY next_try_at, id", store.FormatTime(now))
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
		if err := store.Get(ctx, s.Store.Read(), "releases", *sl.HeldReleaseID, &rel); err != nil || rel.State != "held" {
			continue
		}
		// A season pack held for several Slots is chosen for all of them
		// that still hold it.
		// A Slot missing from the pick list (an old list) is chosen alone.
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

// Search settings (group what_to_pick), each a decision in
// decisions/log-autonomous.md.
const (
	SettingMissingCount  = "missing_search_count"  // Slots per scheduled missing search (default 20)
	SettingMissingBetter = "missing_search_better" // the scheduled search also looks for better copies (default false)
	SettingRespectsGate  = "bulk_search_respects_gate"
	SettingBulkCap       = "bulk_search_cap" // most Slots one bulk search asked by a person queues (default 500)
	defaultBulkCap       = 500
	bulkStagger          = BulkStagger
	// BulkStagger is the time between the searches one bulk search queues.
	BulkStagger = 2 * time.Second
	// SettingArtistSearchCap is the most album searches one Search now on an
	// artist queues (default DefaultArtistSearchCap); the rest are left for
	// the next click or the nightly search.
	SettingArtistSearchCap = "artist_search_cap"
	// Early and fake Releases (4.3.5): a Release posted before its movie,
	// episode or album came out (less the grace hours, default 24) is
	// refused, and one far too small for what it holds; both on unless "false".
	SettingRefuseEarly = "refuse_early_releases"
	SettingEarlyGrace  = "early_release_grace_hours"
	SettingRefuseTiny  = "refuse_tiny_releases"
	// SettingHoldUnknown holds a Release of unknown size or dates for a
	// person's choice in automatic searches ("true"); by default such a
	// Release only gets a warning line (fail-open).
	SettingHoldUnknown = "hold_unknown_releases"
	// SettingEpisodeMinutes is the length taken for an episode of unknown
	// length by the fake-size floor (default DefaultEpisodeMinutes, 1 to 240).
	SettingEpisodeMinutes = "episode_default_minutes"
	// SettingImportFloors turns the fixed least sizes at import off ("false").
	SettingImportFloors    = "import_size_floors"
	DefaultEpisodeMinutes  = 20
	DefaultEarlyGrace      = 24
	DefaultArtistSearchCap = 200
	defaultMissingCount    = 20
	maxMissingCount        = 500
	searchSettingsGroup    = "what_to_pick"
	searchMissingByPerson  = "by_person"
)

// Setting reads one what_to_pick setting ("" when unset).
func Setting(ctx context.Context, db *sql.DB, key string) string {
	var v string
	_ = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", searchSettingsGroup, key).Scan(&v)
	return v
}

// HandleSearchMissing queues a search_slot Job for Slots still looking,
// oldest searched first. Scheduled: up to missing_search_count Slots that
// are wanted (and better_wanted when missing_search_better is on). Params
// from a person: which ("missing" or "better"), up to bulk_search_cap (default
// 500) a run; the searches are queued a little apart. Slots not out
// yet are skipped while bulk_search_respects_gate is on.
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
		// A person asked for the whole list, up to the per-run cap.
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
	skipGate := Setting(ctx, db, SettingRespectsGate) == "false"
	if skipGate && states[0] == string(domain.SlotWanted) {
		states = append(states, string(domain.SlotNotOutYet))
	}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(states)), ", ")
	args := []any{}
	for _, st := range states {
		args = append(args, st)
	}
	// A person may keep a bulk search to one kind (movie, show or artist).
	only, _ := r.Job.Params["kind"].(string)
	args = append(args, only, only)
	// Episodes count when the show and their season are kept up to date
	// too, and only once they are out (setting missing_search_shows, on by
	// default).
	args = append(args, Setting(ctx, db, SettingMissingShows) != "false", Setting(ctx, db, SettingMissingMusic) != "false")
	q := `SELECT s.id, t.kind, COALESCE(i.part_id, 0), s.version_id FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id
		JOIN titles t ON t.id = i.title_id
		WHERE s.state IN (` + ph + `) AND (? = '' OR t.kind = ?) AND i.keep_up_to_date = 1 AND v.keep_up_to_date = 1 AND t.state = 'active' AND s.active_download_id IS NULL
		AND (t.kind = 'movie' OR (? AND t.kind = 'show' AND t.keep_up_to_date = 1 AND s.state != 'not_out_yet' AND i.dropped_at IS NULL
			AND (i.part_id IS NULL OR EXISTS (SELECT 1 FROM parts p WHERE p.id = i.part_id AND p.keep_up_to_date = 1 AND p.dropped_at IS NULL)))
			OR (? AND t.kind = 'artist' AND t.keep_up_to_date = 1 AND s.state != 'not_out_yet' AND i.dropped_at IS NULL
			AND EXISTS (SELECT 1 FROM parts p WHERE p.id = i.part_id AND p.keep_up_to_date = 1 AND p.included = 1 AND p.dropped_at IS NULL)))
		ORDER BY i.last_searched_at IS NOT NULL, i.last_searched_at, s.id`
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
	// Episodes of one season in one Version are searched together, as a
	// season, when there are several; a single one gets an episode search.
	type seasonKey struct{ part, ver int64 }
	perSeason := map[seasonKey]int{}
	for _, f := range list {
		if f.kind == "show" && f.part > 0 {
			perSeason[seasonKey{f.part, f.ver}]++
		}
	}
	queued, seasons, albums := 0, 0, 0
	start := time.Now().UTC()
	seen := map[seasonKey]bool{}
	for _, f := range list {
		params := map[string]any{"bulk": true}
		if skipGate {
			params["skip_gate"] = true
		}
		kind, subject, key := KindSearchSlot, &domain.Subject{Type: "slot", ID: f.id}, "search_slot:"+strconv.FormatInt(f.id, 10)
		if k := (seasonKey{f.part, f.ver}); f.kind == "show" && perSeason[k] > 1 {
			if seen[k] {
				continue
			}
			seen[k] = true
			params["version_id"] = f.ver
			kind, subject = KindSearchSeason, &domain.Subject{Type: "part", ID: f.part}
			key = fmt.Sprintf("search_season:%d:%d", f.part, f.ver)
		}
		if k := (seasonKey{f.part, f.ver}); f.kind == "artist" && f.part > 0 {
			// Tracks are always searched as their album.
			if seen[k] {
				continue
			}
			seen[k] = true
			params["version_id"] = f.ver
			kind, subject = KindSearchAlbum, &domain.Subject{Type: "part", ID: f.part}
			key = fmt.Sprintf("search_album:%d:%d", f.part, f.ver)
		}
		// Spread the searches out so Sources are not asked all at once; the
		// time goes in with the Job, so no worker starts it early.
		if _, created, err := s.Queue.EnqueueAt(ctx, kind, subject, params, key, jobActor(r), start.Add(time.Duration(queued)*bulkStagger)); err != nil {
			return err
		} else if created {
			queued++
			if kind == KindSearchSeason {
				seasons++
			}
			if kind == KindSearchAlbum {
				albums++
			}
		}
	}
	r.SetResult(map[string]any{"slots": len(ids), "queued": queued, "seasons": seasons, "albums": albums})
	return nil
}

// Checks reads the early and fake-size settings for judging.
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

// movieOut is a movie's first known release day: cinema, digital or disc.
func movieOut(t domain.Title) *time.Time {
	var first *time.Time
	for _, d := range []*time.Time{t.CinemaDate, t.DigitalDate, t.PhysicalDate} {
		if d != nil && (first == nil || d.Before(*first)) {
			first = d
		}
	}
	return first
}
