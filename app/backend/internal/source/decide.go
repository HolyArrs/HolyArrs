package source

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/judge"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// Event kinds for searching (stage 3.1).
const (
	EventReleaseChosen     = "release_chosen"
	EventReleaseHeld       = "release_held"
	EventSearchDone        = "search_done"
	EventSearchFailed      = "search_failed"
	EventChosenByHand      = "release_chosen_by_hand"
	EventReleaseTurnedDown = "release_turned_down"
	EventHeldRemoved       = "held_removed"
	EventTurnedDownCleared = "turned_down_removed"
	EventFeedChecked       = "feed_checked"
)

// slotCtx is one Slot with everything judging it needs.
type slotCtx struct {
	sl  domain.Slot
	ver domain.Version
	it  domain.Item
	ti  domain.Title
	pr  domain.PickRules
	cur *judge.Current // the File in place, when there is one
}

func (s *Searcher) loadSlot(ctx context.Context, slotID int64) (slotCtx, error) {
	db := s.Store.Read()
	var c slotCtx
	if err := database.Get(ctx, db, "slots", slotID, &c.sl); err != nil {
		return c, fmt.Errorf("slot %d: %w", slotID, err)
	}
	if err := database.Get(ctx, db, "versions", c.sl.VersionID, &c.ver); err != nil {
		return c, err
	}
	if err := database.Get(ctx, db, "items", c.sl.ItemID, &c.it); err != nil {
		return c, err
	}
	if err := database.Get(ctx, db, "titles", c.it.TitleID, &c.ti); err != nil {
		return c, err
	}
	if err := database.Get(ctx, db, "pick_rules", c.ver.PickRulesID, &c.pr); err != nil {
		return c, err
	}
	// The same reading of the copy in place as the searches (#43).
	c.cur = currentOf(ctx, db, c.sl, c.pr)
	return c, nil
}

func (c slotCtx) ref() judge.TitleRef {
	ref := judge.TitleRef{ID: c.ti.ID, Name: c.ti.Name, Aliases: c.ti.Aliases}
	if c.ti.Year != nil {
		ref.Year = *c.ti.Year
	}
	if c.ti.OriginalLanguage != nil {
		ref.OriginalLanguage = *c.ti.OriginalLanguage
	}
	if c.it.Runtime != nil {
		ref.Runtime = *c.it.Runtime
	}
	return ref
}

// dedupe keeps the first of Releases with the same info hash or name;
// Sources come in priority order, so the better Source's copy stays.
func dedupe(all []found) []found {
	seen := map[string]bool{}
	var out []found
	for _, f := range all {
		keys := []string{"n:" + strings.ToLower(strings.TrimSpace(f.item.Title))}
		if f.item.InfoHash != "" {
			keys = append(keys, "h:"+strings.ToLower(f.item.InfoHash))
		}
		dup := false
		for _, k := range keys {
			dup = dup || seen[k]
		}
		if dup {
			continue
		}
		for _, k := range keys {
			seen[k] = true
		}
		out = append(out, f)
	}
	return out
}

// judgeAll judges every Release for one Slot and ranks them. out says the
// Release gate is open; hand says a person started a single-Item search;
// skipGate says a bulk search may skip the gate (bulk_search_respects_gate off).
func (s *Searcher) judgeAll(ctx context.Context, c slotCtx, all []found, out, hand, skipGate bool) ([]judge.Release, []judge.Verdict, error) {
	bad := s.dangerousSeen(ctx)
	turned, err := s.turnedDown(ctx, c.sl.ID)
	if err != nil {
		return nil, nil, err
	}
	ref := c.ref()
	checks := Checks(ctx, s.Store.Read())
	var outSince *time.Time
	if c.ti.Kind == "" || c.ti.Kind == "movie" {
		outSince = movieOut(c.ti)
	}
	rels := make([]judge.Release, len(all))
	vs := make([]judge.Verdict, len(all))
	for i, f := range all {
		p := releasename.Parse(f.item.Title, releasename.Hints{Size: f.item.Size})
		rels[i] = judge.Release{Name: f.item.Title, Parse: p, Size: f.item.Size, Protocol: f.item.Protocol, Seeders: f.item.Seeders,
			PublishedAt: f.item.Published, InfoHash: f.item.InfoHash, SourcePriority: f.src.Priority, Dangerous: bad.has(listingHash(f.item.InfoHash, f.item.Link), f.item.Title)}
		m := judge.MatchTitle(p, []judge.TitleRef{ref})
		set := judge.SlotSet{Match: m, RuntimeMinutes: ref.Runtime, Out: out, MinSeeders: f.src.MinSeeders, TurnedDown: turned,
			HandSearch: hand, SkipGate: skipGate, Current: c.cur, OutSince: outSince, Checks: checks}
		if m.Title != nil {
			set.SlotIDs = []int64{c.sl.ID}
		}
		vs[i] = copyChoiceVerdict(c, rels[i], judge.Judge(rels[i], set, c.pr.Body))
	}
	judge.Rank(rels, vs, c.pr.Body)
	return rels, vs, nil
}

// overGrabLimit says whether a Source already gave its most picks in its
// window (the picks-per-Source limit, counted by Downloads started from its
// Releases).
func (s *Searcher) overGrabLimit(ctx context.Context, src domain.Source, now time.Time) (bool, string) {
	l := src.Limits
	if l.GrabsPerWindow <= 0 {
		return false, ""
	}
	window := time.Duration(l.WindowSeconds) * time.Second
	if window <= 0 {
		window = 24 * time.Hour
	}
	// Downloads started from its Releases, plus Releases chosen and not yet
	// sent (a pick is a pick before the Downloader has it).
	since := database.FormatTime(now.Add(-window))
	var n int
	err := s.Store.Read().QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM downloads d JOIN releases r ON r.id = d.release_id WHERE r.source_id = ? AND d.created_at > ?) +
		(SELECT count(*) FROM releases r WHERE r.source_id = ? AND r.state = 'chosen' AND r.updated_at > ?
			AND NOT EXISTS (SELECT 1 FROM downloads d WHERE d.release_id = r.id))`, src.ID, since, src.ID, since).Scan(&n)
	if err != nil || n < l.GrabsPerWindow {
		return false, ""
	}
	return true, fmt.Sprintf("Source %s already gave %d picks in the last %s (its limit)", src.Name, n, window)
}

// pickWinner returns the index of the Release to choose or hold, skipping
// Releases from a Source past its picks limit. -1 when none.
func (s *Searcher) pickWinner(ctx context.Context, all []found, vs []judge.Verdict, now time.Time) int {
	order := make([]int, 0, len(vs))
	for r := 1; ; r++ {
		i := -1
		for k := range vs {
			if vs[k].Passed && vs[k].Rank == r {
				i = k
			}
		}
		if i < 0 {
			break
		}
		order = append(order, i)
	}
	for pos, i := range order {
		over, why := s.overGrabLimit(ctx, all[i].src, now)
		if over {
			vs[i].Outcome = "passed_over"
			vs[i].HoldUntil = nil
			vs[i].Reasons = append(vs[i].Reasons, judge.Reason{Step: "rank", Rule: "source_limit", Effect: "rank", Detail: why})
			continue
		}
		if pos > 0 {
			vs[i].Outcome = "chosen"
			if vs[i].HoldUntil != nil && now.Before(*vs[i].HoldUntil) {
				vs[i].Outcome = "held"
			} else {
				vs[i].HoldUntil = nil
			}
			vs[i].Reasons = append(vs[i].Reasons, judge.Reason{Step: "rank", Rule: "first_within_limits", Effect: "rank",
				Detail: "the best Release whose Source is within its picks limit"})
		}
		return i
	}
	return -1
}

// storeAll writes every Release with its verdict for one Slot, or updates
// the stored row of a Release looked at again: its Slots are added to, its
// verdict for this Slot is kept under by_slot, and a chosen Release stays
// chosen. With show, nothing is chosen or held: the winner is kept as
// judged so a person can choose.
func (s *Searcher) storeAll(ctx context.Context, by domain.Actor, jobID *int64, foundBy string, slotID int64, all []found, rels []judge.Release,
	vs []judge.Verdict, show bool) ([]int64, error) {
	ids := make([]int64, len(all))
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for i, f := range all {
			v := vs[i]
			state := v.Outcome
			// A chosen winner stays judged until settleChosen wins the Slot.
			if state == "" || state == "chosen" || show && state == "held" {
				state = "judged"
			}
			hold := v.HoldUntil
			if state != "held" {
				hold = nil
			}
			mine := map[string]any{"outcome": v.Outcome, "passed": v.Passed, "points": v.Points, "rank": v.Rank}
			// The short verdict is kept for each Slot it was judged for (a
			// season pack: every episode it fills).
			bySlot := map[string]any{}
			for _, id := range v.SlotIDs {
				bySlot[strconv.FormatInt(id, 10)] = mine
			}
			if len(bySlot) == 0 {
				bySlot[strconv.FormatInt(slotID, 10)] = mine
			}
			if f.relID > 0 {
				ids[i] = f.relID
				_, err := tx.Exec(`UPDATE releases SET state = CASE WHEN state = 'chosen' THEN state ELSE ? END,
					hold_until = CASE WHEN state = 'chosen' THEN hold_until ELSE ? END,
					verdict = json_set(json(?), '$.by_slot', json_patch(COALESCE(json_extract(verdict, '$.by_slot'), '{}'), json(?))),
					matched_slot_ids = (SELECT json_group_array(value) FROM (SELECT value FROM json_each(matched_slot_ids) UNION SELECT value FROM json_each(?))),
					updated_at = ?, updated_by = ? WHERE id = ?`,
					state, timeOrNil(hold), jsonText(toMap(v)), jsonText(bySlot), jsonText(nonNil(v.SlotIDs)), database.FormatTime(time.Now()), actorText(by), f.relID)
				if err != nil {
					return err
				}
				continue
			}
			verdict := toMap(v)
			verdict["by_slot"] = bySlot
			rel := &domain.Release{SourceID: f.src.ID, Name: f.item.Title, LinkRef: cleanLink(f.item.Link, f.keys), Protocol: f.item.Protocol,
				Size: f.item.Size, Seeders: f.item.Seeders, Peers: f.item.Peers, Flags: rels[i].Parse.Flags,
				Parsed: toMap(rels[i].Parse), MatchedSlotIDs: nonNil(v.SlotIDs), Verdict: verdict, FoundBy: foundBy, JobID: jobID, State: state,
				HoldUntil: hold}
			if rel.Flags == nil {
				rel.Flags = []string{}
			}
			if !f.item.Published.IsZero() {
				p := f.item.Published
				rel.PublishedAt = &p
			}
			if f.item.InfoHash != "" {
				h := f.item.InfoHash
				rel.InfoHash = &h
			}
			id, err := database.Insert(tx, "releases", rel, by)
			if err != nil {
				return err
			}
			ids[i] = id
		}
		return nil
	})
	return ids, err
}

// beforeSettle lets tests change a Slot between judging and settling.
var beforeSettle func(slotID int64)

// pickStates are the states a search or feed check may settle a pick from.
var pickStates = []domain.SlotState{domain.SlotSearching, domain.SlotWanted, domain.SlotBetterWanted, domain.SlotHeld}

// slotStateTx reads a Slot's state inside a write.
func slotStateTx(tx *sql.Tx, slotID int64) (domain.SlotState, error) {
	var from string
	err := tx.QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&from)
	return domain.SlotState(from), err
}

// busyErr says another pick got to the Slot first.
func busyErr(st domain.SlotState) error {
	return fmt.Errorf("%w: it is %s", ErrBusy, strings.ReplaceAll(string(st), "_", " "))
}

// moveSlotTx moves a Slot, going through searching when there is no direct
// arrow (model.md section 4: every pick passes through searching). A chosen
// Slot stays chosen only for the same Release (releaseID already chosen).
func moveSlotTx(tx *sql.Tx, slotID int64, to domain.SlotState, reason string, by domain.Actor, releaseID int64) error {
	if to == domain.SlotChosen {
		// Abandonment is terminal for this Release in this Version. Refuse
		// the choice before it can strand a Slot behind a stopped grab job.
		var abandoned bool
		if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM downloader_attempts a
			JOIN releases r ON r.id = a.release_id AND r.created_at = a.release_created_at
			JOIN versions v ON v.id = a.version_id AND v.created_at = a.version_created_at
			JOIN slots s ON s.version_id = v.id WHERE s.id = ? AND r.id = ? AND a.state = 'abandoned')`, slotID, releaseID).Scan(&abandoned); err != nil {
			return err
		}
		if abandoned {
			return fmt.Errorf("%w: this Release's send was abandoned; choose a different Release", ErrBusy)
		}
	}
	from, err := slotStateTx(tx, slotID)
	if err != nil {
		return err
	}
	if from == to {
		if to == domain.SlotChosen {
			var st string
			if err := tx.QueryRow("SELECT state FROM releases WHERE id = ?", releaseID).Scan(&st); err != nil || st != "chosen" {
				return busyErr(from)
			}
		}
		_, err := tx.Exec("UPDATE slots SET last_reason = ?, updated_at = ?, updated_by = ? WHERE id = ?", reason, database.FormatTime(time.Now()), actorText(by), slotID)
		return err
	}
	if domain.Transition(from, to) != nil && from != domain.SlotSearching {
		if err := store.MoveSlotTx(tx, slotID, domain.SlotSearching, "searching", by); err != nil {
			return err
		}
	}
	if err := store.MoveSlotTx(tx, slotID, to, reason, by); err != nil {
		return err
	}
	if to == domain.SlotChosen {
		// The Download reads which Release the Slot was chosen for.
		_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", releaseID, slotID)
		return err
	}
	return nil
}

// settleChosen makes a Release the Slot's pick in one write: the Release
// is chosen, a Release held before is passed over, the Slot is chosen. The
// Slot's state is read again inside the write; when another pick got there
// first it returns ErrBusy and the Release is passed over.
func (s *Searcher) settleChosen(ctx context.Context, slotID, releaseID int64, name string, by domain.Actor) error {
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		from, err := choiceStateTx(tx, slotID)
		if err != nil {
			return err
		}
		if from != domain.SlotChosen && !slices.Contains(pickStates, from) {
			return busyErr(from)
		}
		if err := moveSlotTx(tx, slotID, domain.SlotChosen, "chose "+name, by, releaseID); err != nil {
			return err
		}
		if err := passOverHeldTx(tx, slotID, releaseID, by); err != nil {
			return err
		}
		return pickReleaseTx(tx, releaseID, "chosen", nil, []int64{slotID}, by)
	})
	if errors.Is(err, ErrBusy) {
		s.lostRace(ctx, releaseID, by)
	}
	return err
}

// settleHeld holds a Release for the Slot until its wait is over; ErrBusy
// when the Slot is no longer looking.
func (s *Searcher) settleHeld(ctx context.Context, slotID, releaseID int64, name string, until time.Time, by domain.Actor) error {
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		from, err := choiceStateTx(tx, slotID)
		if err != nil {
			return err
		}
		if !slices.Contains(pickStates, from) {
			return busyErr(from)
		}
		if err := passOverHeldTx(tx, slotID, releaseID, by); err != nil {
			return err
		}
		if err := moveSlotTx(tx, slotID, domain.SlotHeld, "holding "+name, by, releaseID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET held_release_id = ?, next_try_at = ? WHERE id = ?", releaseID, database.FormatTime(until), slotID); err != nil {
			return err
		}
		return pickReleaseTx(tx, releaseID, "held", &until, []int64{slotID}, by)
	})
	if errors.Is(err, ErrBusy) {
		s.lostRace(ctx, releaseID, by)
	}
	return err
}

// lostRace marks a Release passed over when another pick settled its Slot
// first.
func (s *Searcher) lostRace(ctx context.Context, releaseID int64, by domain.Actor) {
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE releases SET state = 'passed_over', hold_until = NULL, updated_at = ?, updated_by = ?
			WHERE id = ? AND state IN ('judged', 'held')`, database.FormatTime(time.Now()), actorText(by), releaseID)
		return err
	})
}

// passOverHeldTx marks the Slot's Held Release (other than keep) passed
// over and clears the Slot's hold.
func passOverHeldTx(tx *sql.Tx, slotID, keep int64, by domain.Actor) error {
	var held sql.NullInt64
	if err := tx.QueryRow("SELECT held_release_id FROM slots WHERE id = ?", slotID).Scan(&held); err != nil {
		return err
	}
	if held.Valid && held.Int64 != keep {
		if _, err := tx.Exec(`UPDATE releases SET state = 'passed_over', hold_until = NULL, updated_at = ?, updated_by = ?
			WHERE id = ? AND state = 'held'`, database.FormatTime(time.Now()), actorText(by), held.Int64); err != nil {
			return err
		}
	}
	_, err := tx.Exec("UPDATE slots SET held_release_id = NULL, next_try_at = NULL WHERE id = ?", slotID)
	return err
}

// restState is where a Slot goes back to when a search found nothing.
func restState(c slotCtx, from domain.SlotState) domain.SlotState {
	switch {
	case from == domain.SlotNotOutYet:
		return domain.SlotNotOutYet
	case c.cur != nil || from == domain.SlotBetterWanted:
		return domain.SlotBetterWanted
	}
	return domain.SlotWanted
}

func (s *Searcher) addEvent(ctx context.Context, by domain.Actor, jobID *int64, titleID, slotID int64, kind, summary string, data map[string]any) {
	e := &domain.Event{Actor: by, Kind: kind, JobID: jobID, Summary: summary, Data: data, GroupKey: fmt.Sprintf("slot:%d", slotID)}
	if titleID > 0 {
		e.TitleID = &titleID
	}
	if slotID > 0 {
		e.SlotID = &slotID
	}
	if s.Store.AddEvent(ctx, e) == nil {
		s.publish("event", e)
	}
}

func nonNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func actorText(a domain.Actor) string { return jsonText(a) }

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return database.FormatTime(*t)
}
