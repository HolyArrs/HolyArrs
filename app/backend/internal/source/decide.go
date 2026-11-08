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

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

const (
	// Event kinds the search code writes to the event log.
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

// beforeSettle is a test hook called just before a Slot is settled, so a test can make a second pick race this one.
var beforeSettle func(slotID int64)

// pickStates are the Slot states a search may pick from. Anything else means another pick got there first.
var pickStates = []mediafiles.SlotState{mediafiles.SlotSearching, mediafiles.SlotWanted, mediafiles.SlotBetterWanted, mediafiles.SlotHeld}

// slotCtx is a Slot with the Version, Item, Title and pick rules it is judged against, and the copy it holds now.
type slotCtx struct {
	sl  mediafiles.Slot
	ver mediafiles.Version
	it  mediafiles.Item
	ti  mediafiles.Title
	pr  mediafiles.PickRules
	cur *mediafiles.Current
}

// loadSlot reads one Slot with everything judging needs.
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

	c.cur = currentOf(ctx, db, c.sl, c.pr)
	return c, nil
}

// ref describes the Slot's Title for matching release names.
func (c slotCtx) ref() mediafiles.TitleRef {
	ref := mediafiles.TitleRef{ID: c.ti.ID, Name: c.ti.Name, Aliases: c.ti.Aliases}
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

// dedupe drops repeats of one Release across Sources, matching on the lower-cased name or on the info hash.
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

// judgeAll judges and ranks every candidate for one Slot. out says the Slot's item is out, hand marks a person's search, and skipGate ignores the release gate.
func (s *Searcher) judgeAll(ctx context.Context, c slotCtx, all []found, out, hand, skipGate bool) ([]mediafiles.ReleaseFacts, []mediafiles.Verdict, error) {
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
	rels := make([]mediafiles.ReleaseFacts, len(all))
	vs := make([]mediafiles.Verdict, len(all))
	for i, f := range all {
		p := releasename.Parse(f.item.Title, releasename.Hints{Size: f.item.Size})
		rels[i] = mediafiles.ReleaseFacts{Name: f.item.Title, Parse: p, Size: f.item.Size, Protocol: f.item.Protocol, Seeders: f.item.Seeders,
			PublishedAt: f.item.Published, InfoHash: f.item.InfoHash, SourcePriority: f.src.Priority, Dangerous: bad.has(listingHash(f.item.InfoHash, f.item.Link), f.item.Title)}
		m := mediafiles.MatchTitle(p, []mediafiles.TitleRef{ref})
		set := mediafiles.SlotSet{Match: m, RuntimeMinutes: ref.Runtime, Out: out, MinSeeders: f.src.MinSeeders, TurnedDown: turned,
			HandSearch: hand, SkipGate: skipGate, Current: c.cur, OutSince: outSince, Checks: checks}
		if m.Title != nil {
			set.SlotIDs = []int64{c.sl.ID}
		}
		vs[i] = copyChoiceVerdict(c, rels[i], mediafiles.Judge(rels[i], set, c.pr.Body))
	}
	mediafiles.Rank(rels, vs, c.pr.Body)
	return rels, vs, nil
}

// overGrabLimit reports whether the Source has already given its picks for the current window, and says why in plain words. Picks not yet sent to a downloader count too, so a burst cannot pass the limit.
func (s *Searcher) overGrabLimit(ctx context.Context, src domain.Source, now time.Time) (bool, string) {
	l := src.Limits
	if l.GrabsPerWindow <= 0 {
		return false, ""
	}
	// A limit without a window counts a day.
	window := time.Duration(l.WindowSeconds) * time.Second
	if window <= 0 {
		window = 24 * time.Hour
	}

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

// pickWinner returns the index of the best Release whose Source is within its limit, or -1. Releases ranked above it that were skipped for a limit are marked passed over, and the winner is marked chosen or held when it was not the top rank.
func (s *Searcher) pickWinner(ctx context.Context, all []found, vs []mediafiles.Verdict, now time.Time) int {
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
			vs[i].Reasons = append(vs[i].Reasons, mediafiles.Reason{Step: "rank", Rule: "source_limit", Effect: "rank", Detail: why})
			continue
		}
		if pos > 0 {
			vs[i].Outcome = "chosen"
			if vs[i].HoldUntil != nil && now.Before(*vs[i].HoldUntil) {
				vs[i].Outcome = "held"
			} else {
				vs[i].HoldUntil = nil
			}
			vs[i].Reasons = append(vs[i].Reasons, mediafiles.Reason{Step: "rank", Rule: "first_within_limits", Effect: "rank",
				Detail: "the best Release whose Source is within its picks limit"})
		}
		return i
	}
	return -1
}

// storeAll saves every judged candidate in one write and returns the Release IDs in the same order. A candidate already stored is updated, never copied. A show Release held for a person to choose stays judged here, so each Slot keeps its own say.
func (s *Searcher) storeAll(ctx context.Context, by domain.Actor, jobID *int64, foundBy string, slotID int64, all []found, rels []mediafiles.ReleaseFacts,
	vs []mediafiles.Verdict, show bool) ([]int64, error) {
	ids := make([]int64, len(all))
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for i, f := range all {
			v := vs[i]
			state := v.Outcome

			// A chosen verdict is still only judged here. The Slot settles later and marks the Release chosen.
			if state == "" || state == "chosen" || show && state == "held" {
				state = "judged"
			}
			hold := v.HoldUntil
			if state != "held" {
				hold = nil
			}
			mine := map[string]any{"outcome": v.Outcome, "passed": v.Passed, "points": v.Points, "rank": v.Rank}

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

// slotStateTx reads a Slot's state inside the caller's write.
func slotStateTx(tx *sql.Tx, slotID int64) (mediafiles.SlotState, error) {
	var from string
	err := tx.QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&from)
	return mediafiles.SlotState(from), err
}

// busyErr is the error for a Slot that is no longer in a state a pick can use.
func busyErr(st mediafiles.SlotState) error {
	return fmt.Errorf("%w: it is %s", ErrBusy, strings.ReplaceAll(string(st), "_", " "))
}

// moveSlotTx moves a Slot to a new state and records why. Choosing a Release whose send was abandoned is refused, and a move that skips the searching state goes through it first.
func moveSlotTx(tx *sql.Tx, slotID int64, to mediafiles.SlotState, reason string, by domain.Actor, releaseID int64) error {
	if to == mediafiles.SlotChosen {

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
		if to == mediafiles.SlotChosen {
			var st string
			if err := tx.QueryRow("SELECT state FROM releases WHERE id = ?", releaseID).Scan(&st); err != nil || st != "chosen" {
				return busyErr(from)
			}
		}
		_, err := tx.Exec("UPDATE slots SET last_reason = ?, updated_at = ?, updated_by = ? WHERE id = ?", reason, database.FormatTime(time.Now()), actorText(by), slotID)
		return err
	}
	if mediafiles.Transition(from, to) != nil && from != mediafiles.SlotSearching {
		if err := mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotSearching, "searching", by); err != nil {
			return err
		}
	}
	if err := mediafiles.MoveSlotTx(tx, slotID, to, reason, by); err != nil {
		return err
	}
	if to == mediafiles.SlotChosen {

		_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", releaseID, slotID)
		return err
	}
	return nil
}

// settleChosen makes the Release the Slot's pick. It fails with ErrBusy when the Slot, its Title or its copy no longer allows it, and then the Release is marked passed over.
func (s *Searcher) settleChosen(ctx context.Context, slotID, releaseID int64, name string, by domain.Actor) error {
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		from, err := choiceStateTx(tx, slotID)
		if err != nil {
			return err
		}
		if from != mediafiles.SlotChosen && !slices.Contains(pickStates, from) {
			return busyErr(from)
		}
		if err := moveSlotTx(tx, slotID, mediafiles.SlotChosen, "chose "+name, by, releaseID); err != nil {
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

// settleHeld holds the Release for the Slot until the given time. It fails with ErrBusy when the Slot no longer allows it.
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
		if err := moveSlotTx(tx, slotID, mediafiles.SlotHeld, "holding "+name, by, releaseID); err != nil {
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

// lostRace marks a Release that lost its Slot to another pick as passed over, unless it is already settled.
func (s *Searcher) lostRace(ctx context.Context, releaseID int64, by domain.Actor) {
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE releases SET state = 'passed_over', hold_until = NULL, updated_at = ?, updated_by = ?
			WHERE id = ? AND state IN ('judged', 'held')`, database.FormatTime(time.Now()), actorText(by), releaseID)
		return err
	})
}

// passOverHeldTx passes over the Slot's held Release unless it is the one to keep, and clears the hold.
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

// restState returns the state a Slot goes back to when it stays unsettled.
func restState(c slotCtx, from mediafiles.SlotState) mediafiles.SlotState {
	switch {
	case from == mediafiles.SlotNotOutYet:
		return mediafiles.SlotNotOutYet
	case c.cur != nil || from == mediafiles.SlotBetterWanted:
		return mediafiles.SlotBetterWanted
	}
	return mediafiles.SlotWanted
}

// addEvent writes an event and publishes it. A failed write is not published.
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

// nonNil turns a nil list into an empty one so it is stored as [] not null.
func nonNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

// jsonText encodes a value as JSON text for a SQL argument.
func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// actorText encodes an actor as stored in updated_by.
func actorText(a domain.Actor) string { return jsonText(a) }

// timeOrNil formats a time for SQL, or returns nil for a missing one.
func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return database.FormatTime(*t)
}
