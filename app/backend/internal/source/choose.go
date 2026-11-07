package source

import (
	"context"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/judge"
	"github.com/CoderLuii/holyarrs/internal/parse"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Errors a person's choice can meet.
var (
	ErrNoRelease = errors.New("no such Release")
	ErrNoSlot    = errors.New("say which Slot: the Release matched none")
	ErrBusy      = errors.New("the Slot is busy")
	ErrState     = errors.New("the Release is not in that list")
)

// NeedsForceError says a refused or turned-down Release needs force to be
// chosen, with the reasons it failed.
type NeedsForceError struct{ Reasons []string }

func (e *NeedsForceError) Error() string {
	return "this Release failed the Pick rules (" + strings.Join(e.Reasons, "; ") + "); choose it with force to send it anyway"
}

// Choice is what choosing a Release by hand did.
type Choice struct {
	ReleaseID int64    `json:"release_id"`
	SlotID    int64    `json:"slot_id"`
	SlotIDs   []int64  `json:"slot_ids,omitempty"` // a season pack or multi-episode Release: every Slot it was chosen for
	Forced    bool     `json:"forced"`
	Warnings  []string `json:"warnings"`
}

// failReasons lists a stored verdict's failed reasons in plain words.
func failReasons(rel domain.Release) []string {
	var out []string
	rs, _ := rel.Verdict["reasons"].([]any)
	for _, x := range rs {
		m, _ := x.(map[string]any)
		if m["effect"] == "fail" {
			if d, _ := m["detail"].(string); d != "" {
				out = append(out, d)
			}
		}
	}
	if rel.State == "turned_down" && len(TurnedDownSlots(rel)) == 0 {
		why := "it was turned down"
		if d, _ := rel.Verdict["turned_down_reason"].(string); d != "" {
			why += ": " + d
		}
		out = append(out, why)
	}
	if len(out) == 0 {
		out = []string{"it did not pass"}
	}
	return out
}

// canTakePick says a Slot may be given a Release by hand: not busy, not
// removed, and with a way to chosen (directly or through searching).
func canTakePick(st domain.SlotState) bool {
	return !busySlot(st) && st != domain.SlotRemoved &&
		(domain.Transition(st, domain.SlotChosen) == nil || domain.Transition(st, domain.SlotSearching) == nil)
}

// busySlot says a Slot is already getting a copy.
func busySlot(st domain.SlotState) bool {
	return st == domain.SlotChosen || st == domain.SlotDownloading || st == domain.SlotImporting || st == domain.SlotSearching
}

// Choose sends a Release a person picked to its Slot (slotID 0: the Slot it
// matched). A refused or turned-down Release needs force; the reasons it
// failed are kept on the Event. Choosing a Release for a Title that is not
// out yet works, with a warning.
func (s *Searcher) Choose(ctx context.Context, releaseID, slotID int64, force bool, by domain.Actor) (Choice, error) {
	db := s.Store.Read()
	var rel domain.Release
	if err := store.Get(ctx, db, "releases", releaseID, &rel); err != nil {
		return Choice{}, ErrNoRelease
	}
	if slotID == 0 && len(rel.MatchedSlotIDs) > 1 {
		many, skipped, kind := s.packSlots(ctx, rel)
		if len(many) > 1 || len(many) == 1 && len(skipped) > 0 {
			return s.chooseMany(ctx, rel, many, skipped, kind == "artist", force, nil, by)
		}
		if len(many) == 0 && len(skipped) > 0 {
			// Every Slot already has a copy as good (or a protected one):
			// taking the Release over them would be no gain, which needs
			// force like a refused Release (#43: an MP3 album over FLAC).
			// With force it goes to every Slot without a protected copy.
			why := []string{"nothing would gain: " + strings.Join(skipped, "; ")}
			if !force {
				return Choice{}, &NeedsForceError{Reasons: why}
			}
			if open := s.unprotectedSlots(ctx, rel); len(open) > 0 {
				return s.chooseMany(ctx, rel, open, nil, kind == "artist", true, why, by)
			}
		}
	}
	if slotID == 0 {
		if len(rel.MatchedSlotIDs) == 0 {
			return Choice{}, ErrNoSlot
		}
		slotID = rel.MatchedSlotIDs[0]
		for _, id := range rel.MatchedSlotIDs {
			if !turnedDownFor(rel, id) {
				slotID = id
				break
			}
		}
	}
	c, err := s.loadSlot(ctx, slotID)
	if err != nil {
		return Choice{}, ErrNoSlot
	}
	if !canTakePick(c.sl.State) {
		return Choice{}, busyErr(c.sl.State)
	}
	ch := Choice{ReleaseID: rel.ID, SlotID: slotID, Warnings: []string{}}
	refused := RefusedByRules(rel) || turnedDownFor(rel, slotID) || len(rel.MatchedSlotIDs) > 0 && !slices.Contains(rel.MatchedSlotIDs, slotID)
	var overridden []string
	if s.dangerousRel(ctx, rel) {
		refused = true
		overridden = append(overridden, dangerousWhy)
	}
	// A track whose copy in place is as good (an MP3 over FLAC) gains
	// nothing: that needs force too (#43, Lossless protection).
	if c.cur != nil && domain.IsMusicRules(c.pr.Body) {
		jr := judge.Release{Name: rel.Name, Parse: parse.Parse(rel.Name, parse.Hints{Size: rel.Size, Music: true}), Size: rel.Size, Protocol: rel.Protocol}
		if !judge.Gains(jr, c.pr.Body, c.cur) {
			refused = true
			overridden = append(overridden, trackName(c.it)+" already has a copy as good")
		}
	}
	if refused {
		overridden = append(failReasons(rel), overridden...)
		if turnedDownFor(rel, slotID) && len(TurnedDownSlots(rel)) > 0 {
			why := "it was turned down for this episode"
			if d, _ := rel.Verdict["turned_down_reason"].(string); d != "" {
				why += ": " + d
			}
			overridden = append(overridden, why)
		}
		if !slices.Contains(rel.MatchedSlotIDs, slotID) && len(rel.MatchedSlotIDs) > 0 {
			overridden = append(overridden, "it was judged for another Slot")
		}
		if !force {
			return Choice{}, &NeedsForceError{Reasons: overridden}
		}
		ch.Forced = true
		ch.Warnings = append(ch.Warnings, "sent although it failed: "+strings.Join(overridden, "; "))
	}
	if c.sl.State == domain.SlotNotOutYet {
		ch.Warnings = append(ch.Warnings, "the Release gate is skipped: this Title is not out yet")
	}
	if c.sl.State == domain.SlotInLibrary {
		ch.Warnings = append(ch.Warnings, "this Version already has a good enough copy; the new one will replace it")
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		// Read again inside the write: a search or feed check may have
		// settled the Slot since it was loaded.
		st, err := choiceStateTx(tx, slotID)
		if err != nil {
			return err
		}
		if !canTakePick(st) {
			return busyErr(st)
		}
		if err := passOverHeldTx(tx, slotID, rel.ID, by); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE releases SET state = 'chosen', hold_until = NULL,
			matched_slot_ids = CASE WHEN EXISTS (SELECT 1 FROM json_each(matched_slot_ids) WHERE value = ?) THEN matched_slot_ids
				ELSE json_insert(matched_slot_ids, '$[#]', ?) END,
			updated_at = ?, updated_by = ? WHERE id = ?`, slotID, slotID, store.FormatTime(time.Now()), actorText(by), rel.ID); err != nil {
			return err
		}
		// Chosen with force for an episode it was turned down for: it leaves
		// the Turned down list for that episode.
		if only := TurnedDownSlots(rel); slices.Contains(only, slotID) {
			if err := setTurnedDownSlotsTx(tx, rel.ID, slices.DeleteFunc(slices.Clone(only), func(x int64) bool { return x == slotID })); err != nil {
				return err
			}
		}
		return moveSlotTx(tx, slotID, domain.SlotChosen, "chosen by hand: "+rel.Name, by, rel.ID)
	})
	if err != nil {
		return Choice{}, err
	}
	data := map[string]any{"release_id": rel.ID, "forced": ch.Forced, "warnings": ch.Warnings}
	if len(overridden) > 0 {
		data["reasons_overridden"] = overridden
	}
	s.addEvent(ctx, by, nil, c.ti.ID, slotID, EventChosenByHand, "Chosen by hand: "+rel.Name, data)
	s.publish("slot", map[string]any{"id": slotID, "state": domain.SlotChosen, "reason": "chosen by hand: " + rel.Name})
	if s.OnChosen != nil {
		s.OnChosen(ctx, slotID, rel.ID)
	}
	return ch, nil
}

// TurnDown marks a Release turned down: it is never picked again for its
// Slots. A Held Release leaves its Slot, which goes back to looking.
func (s *Searcher) TurnDown(ctx context.Context, releaseID int64, reason string, by domain.Actor) error {
	return s.TurnDownFor(ctx, releaseID, 0, reason, by)
}

// TurnDownFor turns a Release down for one Slot only (slotID > 0): a
// season pack turned down for one episode stays on the list for that
// episode and is still judged for the others. slotID 0 turns it down for
// every Slot it matched.
func (s *Searcher) TurnDownFor(ctx context.Context, releaseID, slotID int64, reason string, by domain.Actor) error {
	var rel domain.Release
	if err := store.Get(ctx, s.Store.Read(), "releases", releaseID, &rel); err != nil {
		return ErrNoRelease
	}
	if slotID > 0 && len(rel.MatchedSlotIDs) > 0 && !slices.Contains(rel.MatchedSlotIDs, slotID) {
		return ErrNoSlot
	}
	if reason == "" {
		reason = "turned down by hand"
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE releases SET state = 'turned_down', hold_until = NULL, verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?),
			updated_at = ?, updated_by = ? WHERE id = ?`, reason, store.FormatTime(time.Now()), actorText(by), rel.ID); err != nil {
			return err
		}
		if slotID > 0 {
			ids := append(TurnedDownSlots(rel), slotID)
			if _, err := tx.Exec(`UPDATE releases SET verdict = json_set(verdict, '$.turned_down_slot_ids', json(?)) WHERE id = ?`, jsonText(ids), rel.ID); err != nil {
				return err
			}
		}
		return unholdTx(tx, rel.ID, "Held Release turned down", by)
	})
	if err != nil {
		return err
	}
	slots := rel.MatchedSlotIDs
	if slotID > 0 {
		slots = []int64{slotID}
	}
	for _, sid := range slots {
		s.addEvent(ctx, by, nil, 0, sid, EventReleaseTurnedDown, "Turned down "+rel.Name+": "+reason, map[string]any{"release_id": rel.ID})
	}
	s.publish("releases", map[string]any{"release_id": rel.ID, "state": "turned_down"})
	return nil
}

// unholdTx sends Slots holding this Release back to looking.
func unholdTx(tx *sql.Tx, releaseID int64, reason string, by domain.Actor) error {
	rows, err := tx.Query("SELECT id, file_id IS NOT NULL FROM slots WHERE held_release_id = ? AND state = 'held'", releaseID)
	if err != nil {
		return err
	}
	type hs struct {
		id      int64
		hasFile bool
	}
	var list []hs
	for rows.Next() {
		var h hs
		if err := rows.Scan(&h.id, &h.hasFile); err != nil {
			rows.Close()
			return err
		}
		list = append(list, h)
	}
	rows.Close()
	for _, h := range list {
		to := domain.SlotWanted
		if h.hasFile {
			to = domain.SlotBetterWanted
		}
		if _, err := tx.Exec("UPDATE slots SET held_release_id = NULL, next_try_at = NULL WHERE id = ?", h.id); err != nil {
			return err
		}
		if err := store.MoveSlotTx(tx, h.id, to, reason, by); err != nil {
			return err
		}
	}
	return nil
}

// RemoveHeld takes Held Releases off the list (id 0: all of them). Each
// becomes passed over and its Slot goes back to looking. It returns how
// many were removed.
func (s *Searcher) RemoveHeld(ctx context.Context, id int64, by domain.Actor) (int, error) {
	n := 0
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		q := "SELECT id FROM releases WHERE state = 'held'"
		args := []any{}
		if id > 0 {
			q += " AND id = ?"
			args = append(args, id)
		}
		ids, err := scanIDs(tx.Query(q, args...))
		if err != nil {
			return err
		}
		if id > 0 && len(ids) == 0 {
			return ErrState
		}
		for _, rid := range ids {
			if _, err := tx.Exec(`UPDATE releases SET state = 'passed_over', hold_until = NULL, verdict = json_set(COALESCE(verdict, '{}'), '$.removed_by_hand', json('true')),
				updated_at = ?, updated_by = ? WHERE id = ?`, store.FormatTime(time.Now()), actorText(by), rid); err != nil {
				return err
			}
			if err := unholdTx(tx, rid, "Held Release removed by hand", by); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err == nil && n > 0 {
		s.addEvent(ctx, by, nil, 0, 0, EventHeldRemoved, fmt.Sprintf("Removed %d Held Releases", n), map[string]any{"count": n, "release_id": id})
		s.publish("releases", map[string]any{"held_removed": n})
	}
	return n, err
}

// RemoveTurnedDown takes Releases off the Turned down list (id 0: all), so
// they may be picked again. The rows are deleted, like an old block list
// entry being removed; a Release a Download came from is kept as passed
// over instead (or refused, when the Pick rules refused it), so the
// Download keeps its Release and its Source's picks.
func (s *Searcher) RemoveTurnedDown(ctx context.Context, id int64, by domain.Actor) (int, error) {
	n := 0
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		where := "state = 'turned_down'"
		args := []any{}
		if id > 0 {
			where += " AND id = ?"
			args = append(args, id)
		}
		// A Release turned down for single episodes and chosen for others
		// meanwhile only forgets those episodes.
		res, err := tx.Exec(`UPDATE releases SET verdict = json_remove(verdict, '$.turned_down_slot_ids'), updated_at = ?, updated_by = ?
			WHERE state != 'turned_down' AND json_extract(verdict, '$.turned_down_slot_ids') IS NOT NULL`+strings.TrimPrefix(where, "state = 'turned_down'"),
			append([]any{store.FormatTime(time.Now()), actorText(by)}, args...)...)
		if err != nil {
			return err
		}
		forgot, _ := res.RowsAffected()
		n += int(forgot)
		used := " AND (EXISTS (SELECT 1 FROM downloads d WHERE d.release_id = releases.id) OR EXISTS (SELECT 1 FROM downloader_attempts a WHERE a.release_id = releases.id))"
		// One the Pick rules refused (a forced .exe, say) stays refused, so
		// picking it again still needs force (#41).
		res, err = tx.Exec(`UPDATE releases SET state = CASE WHEN json_extract(verdict, '$.outcome') = 'refused' AND EXISTS (SELECT 1 FROM json_each(verdict, '$.reasons') rr
				WHERE json_extract(rr.value, '$.effect') = 'fail' AND NOT (json_extract(rr.value, '$.step') = 'hard' AND json_extract(rr.value, '$.rule') = 'turned_down'))
				OR EXISTS (SELECT 1 FROM downloads dd WHERE dd.release_id = releases.id AND dd.state = 'failed' AND dd.state_reason LIKE 'dangerous files%')
				THEN 'refused' ELSE 'passed_over' END,
			verdict = json_set(COALESCE(verdict, '{}'), '$.taken_off_turned_down', json('true')),
			updated_at = ?, updated_by = ? WHERE `+where+used, append([]any{store.FormatTime(time.Now()), actorText(by)}, args...)...)
		if err != nil {
			return err
		}
		kept, _ := res.RowsAffected()
		res, err = tx.Exec("DELETE FROM releases WHERE "+where, args...)
		if err != nil {
			return err
		}
		k, _ := res.RowsAffected()
		n += int(k + kept)
		if id > 0 && n == 0 {
			return ErrState
		}
		return nil
	})
	if err == nil && n > 0 {
		s.addEvent(ctx, by, nil, 0, 0, EventTurnedDownCleared, fmt.Sprintf("Took %d Releases off the Turned down list", n), map[string]any{"count": n, "release_id": id})
		s.publish("releases", map[string]any{"turned_down_removed": n})
	}
	return n, err
}

func scanIDs(rows *sql.Rows, err error) ([]int64, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListedRelease is a Release on the Held or Turned down list, with the
// Title and Source it belongs to.
type ListedRelease struct {
	domain.Release
	SourceName string
	TitleID    int64
	TitleName  string
	SlotID     int64
	Version    string
}

// List pages through Releases in one state (held or turned_down), newest
// first, and says how many there are.
func (s *Searcher) List(ctx context.Context, state string, limit, offset int) ([]ListedRelease, int, error) {
	db := s.Store.Read()
	var total int
	where, args := "state = ?", []any{state}
	if state == "turned_down" {
		where, args = turnedDownList, nil
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM releases WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "updated_at DESC, id DESC"
	if state == "held" {
		order = "hold_until, id"
	}
	rels, err := store.Select[domain.Release](ctx, db, "releases", where+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ListedRelease, 0, len(rels))
	for _, r := range rels {
		l := ListedRelease{Release: r}
		_ = db.QueryRowContext(ctx, "SELECT name FROM sources WHERE id = ?", r.SourceID).Scan(&l.SourceName)
		if only := TurnedDownSlots(r); state == "turned_down" && len(only) > 0 {
			l.SlotID = only[0]
		} else if len(r.MatchedSlotIDs) > 0 {
			l.SlotID = r.MatchedSlotIDs[0]
			_ = db.QueryRowContext(ctx, `SELECT t.id, t.name, v.label FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
				JOIN versions v ON v.id = s.version_id WHERE s.id = ?`, l.SlotID).Scan(&l.TitleID, &l.TitleName, &l.Version)
		}
		out = append(out, l)
	}
	return out, total, nil
}

// TurnedDownSlots lists the Slots a Release was turned down for one by one
// (verdict.turned_down_slot_ids); nil when it was turned down for all.
func TurnedDownSlots(rel domain.Release) []int64 {
	xs, _ := rel.Verdict["turned_down_slot_ids"].([]any)
	var out []int64
	for _, x := range xs {
		if f, ok := x.(float64); ok && !slices.Contains(out, int64(f)) {
			out = append(out, int64(f))
		}
	}
	return out
}

// turnedDownFor says a Release is turned down for a Slot. A Release turned
// down for single episodes stays so for them whatever its state (it may be
// chosen for other episodes meanwhile).
func turnedDownFor(rel domain.Release, slotID int64) bool {
	if only := TurnedDownSlots(rel); len(only) > 0 {
		return slices.Contains(only, slotID)
	}
	return rel.State == "turned_down"
}

// turnedDownList is the SQL condition for the Turned down list: turned
// down for every Slot, or for single episodes.
const turnedDownList = "(state = 'turned_down' OR json_extract(verdict, '$.turned_down_slot_ids') IS NOT NULL)"

// packSlots lists the Slots of a show Release a person may choose it for
// at once: the Slots it matched that can take a pick and that it was not
// turned down for. An episode with a copy in place is taken only when the
// Release would be a better copy; skipped names those left out.
func (s *Searcher) packSlots(ctx context.Context, rel domain.Release) (out []int64, skipped []string, kind string) {
	_ = s.Store.Read().QueryRowContext(ctx, `SELECT t.kind FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE s.id = ?`, rel.MatchedSlotIDs[0]).Scan(&kind)
	if kind != "show" && kind != "artist" {
		return nil, nil, kind
	}
	jr := judge.Release{Name: rel.Name, Parse: parse.Parse(rel.Name, parse.Hints{Size: rel.Size, Show: kind == "show", Music: kind == "artist"}), Size: rel.Size, Protocol: rel.Protocol}
	for _, id := range rel.MatchedSlotIDs {
		var st string
		if s.Store.Read().QueryRowContext(ctx, "SELECT state FROM slots WHERE id = ?", id).Scan(&st) != nil ||
			!canTakePick(domain.SlotState(st)) || turnedDownFor(rel, id) {
			continue
		}
		if slotHasProtectedCopy(ctx, s.Store.Read(), id) {
			if c, err := s.loadSlot(ctx, id); err == nil {
				what := episodeLabel(c.it)
				if kind == "artist" {
					what = trackName(c.it)
				}
				skipped = append(skipped, what+" has a protected copy")
			}
			continue
		}
		if c, err := s.loadSlot(ctx, id); err == nil && c.cur != nil && !judge.Gains(jr, c.pr.Body, c.cur) {
			what := episodeLabel(c.it)
			if kind == "artist" {
				what = trackName(c.it)
			}
			skipped = append(skipped, what+" already has a copy as good")
			continue
		}
		out = append(out, id)
	}
	return out, skipped, kind
}

// RefusedByRules says whether the Pick rules refused a Release, whatever its
// state now: one sent with force and then removed, or taken off the Turned
// down list, still needs force to be picked again (#41). A verdict whose
// only failing reason is that it was turned down before does not count:
// clearing the list clears that.
func RefusedByRules(rel domain.Release) bool {
	if rel.State == "refused" || rel.Verdict["dangerous_files"] == true {
		return true
	}
	if o, _ := rel.Verdict["outcome"].(string); o != "refused" {
		return false
	}
	reasons, _ := rel.Verdict["reasons"].([]any)
	if len(reasons) == 0 {
		return true // an older verdict without its reasons
	}
	for _, x := range reasons {
		r, _ := x.(map[string]any)
		if r["effect"] == "fail" && !(r["step"] == "hard" && r["rule"] == "turned_down") {
			return true
		}
	}
	return false
}

// dangerousRels are the info hashes and names of Releases whose Download
// failed because its files were a program (fail_dangerous). The Downloads
// stay as history, so this outlives clearing the Turned down list.
type dangerousRels struct{ hashes, names map[string]bool }

// has matches by info hash; by name only when there is no hash (a fake
// may copy a real scene name, and the real torrent must stay free).
func (d dangerousRels) has(hash, name string) bool {
	if hash != "" {
		return d.hashes[strings.ToLower(hash)]
	}
	return name != "" && d.names[name]
}

var magnetBtih = regexp.MustCompile(`(?i)[?&]xt=urn:btih:([0-9a-f]{40}|[a-z2-7]{32})(?:&|$)`)

// listingHash is a listing's info hash, from its own field or its magnet
// link (hex, or base32 as some sites write it).
func listingHash(hash, link string) string {
	if hash != "" {
		return hash
	}
	m := magnetBtih.FindStringSubmatch(link)
	if m == nil {
		return ""
	}
	if len(m[1]) == 40 {
		return strings.ToLower(m[1])
	}
	b, err := base32.StdEncoding.DecodeString(strings.ToUpper(m[1]))
	if err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// dangerousWhy is the reason a Release known to hold a program needs force.
const dangerousWhy = "its files were a program when it was downloaded before"

// dangerousRel says whether a Download of rel (by info hash or name) failed
// because its files were a program: picking it again needs force, whatever
// its stored verdict says (#41).
func (s *Searcher) dangerousRel(ctx context.Context, rel domain.Release) bool {
	h := ""
	if rel.InfoHash != nil {
		h = *rel.InfoHash
	}
	if d := s.dangerousSeen(ctx); d.has(listingHash(h, rel.LinkRef), rel.Name) {
		return true
	}
	var n int
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM downloads WHERE release_id = ? AND state = 'failed' AND state_reason LIKE 'dangerous files%'", rel.ID).Scan(&n)
	return n > 0
}

func (s *Searcher) dangerousSeen(ctx context.Context) dangerousRels {
	d := dangerousRels{hashes: map[string]bool{}, names: map[string]bool{}}
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT lower(d.external_id), COALESCE(r.name, '') FROM downloads d LEFT JOIN releases r ON r.id = d.release_id
		WHERE d.state = 'failed' AND d.state_reason LIKE 'dangerous files%'`)
	if err != nil {
		return d
	}
	defer rows.Close()
	for rows.Next() {
		var h, n string
		if rows.Scan(&h, &n) == nil {
			if ValidInfoHash(h) {
				d.hashes[h] = true
			}
			if n != "" {
				d.names[n] = true
			}
		}
	}
	return d
}

// unprotectedSlots are the Slots a Release matched that can take a pick, that
// it was not turned down for and that hold no protected copy: where a
// forced pick of a Release that gains nothing goes.
func (s *Searcher) unprotectedSlots(ctx context.Context, rel domain.Release) []int64 {
	var out []int64
	for _, id := range rel.MatchedSlotIDs {
		var st string
		if s.Store.Read().QueryRowContext(ctx, "SELECT state FROM slots WHERE id = ?", id).Scan(&st) != nil ||
			!canTakePick(domain.SlotState(st)) || turnedDownFor(rel, id) || slotHasProtectedCopy(ctx, s.Store.Read(), id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// episodeLabel names an episode as S01E02.
func episodeLabel(it domain.Item) string {
	if it.Numbers.Season == nil || it.Numbers.Episode == nil {
		return "an episode"
	}
	return fmt.Sprintf("S%02dE%02d", *it.Numbers.Season, *it.Numbers.Episode)
}

// chooseMany sends a season pack or multi-episode Release a person picked
// to every Slot it fills, as one Download. For an album (album), leaving
// tracks out would mix copies within it: that needs force, like a refused
// Release.
func (s *Searcher) chooseMany(ctx context.Context, rel domain.Release, slots []int64, skipped []string, album, force bool, also []string, by domain.Actor) (Choice, error) {
	ch := Choice{ReleaseID: rel.ID, SlotID: slots[0], Warnings: []string{}}
	if len(skipped) > 0 {
		ch.Warnings = append(ch.Warnings, "left out: "+strings.Join(skipped, "; "))
	}
	var overridden []string
	if RefusedByRules(rel) {
		overridden = failReasons(rel)
	}
	if s.dangerousRel(ctx, rel) {
		overridden = append(overridden, dangerousWhy)
	}
	if album && len(skipped) > 0 {
		overridden = append(overridden, "the album would mix copies: "+strings.Join(skipped, "; "))
	}
	overridden = append(overridden, also...)
	if len(overridden) > 0 {
		if !force {
			return Choice{}, &NeedsForceError{Reasons: overridden}
		}
		ch.Forced = true
		ch.Warnings = append(ch.Warnings, "sent although it failed: "+strings.Join(overridden, "; "))
	}
	const notAired = "the Release gate is skipped: some episodes have not aired yet"
	var took []int64
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range slots {
			st, err := choiceStateTx(tx, id)
			if err != nil {
				return err
			}
			if !canTakePick(st) {
				continue // got busy meanwhile
			}
			if st == domain.SlotNotOutYet && !slices.Contains(ch.Warnings, notAired) {
				ch.Warnings = append(ch.Warnings, notAired)
			}
			if err := passOverHeldTx(tx, id, rel.ID, by); err != nil {
				return err
			}
			if err := moveSlotTx(tx, id, domain.SlotChosen, "chosen by hand: "+rel.Name, by, rel.ID); err != nil {
				return err
			}
			took = append(took, id)
		}
		if len(took) == 0 {
			return busyErr(domain.SlotChosen)
		}
		return pickReleaseTx(tx, rel.ID, "chosen", nil, took, by)
	})
	if err != nil {
		return Choice{}, err
	}
	ch.SlotID, ch.SlotIDs = took[0], took
	data := map[string]any{"release_id": rel.ID, "forced": ch.Forced, "warnings": ch.Warnings, "slot_ids": took}
	if len(overridden) > 0 {
		data["reasons_overridden"] = overridden
	}
	var titleID int64
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", took[0]).Scan(&titleID)
	for _, id := range took {
		s.addEvent(ctx, by, nil, titleID, id, EventChosenByHand, "Chosen by hand: "+rel.Name, data)
		s.publish("slot", map[string]any{"id": id, "state": domain.SlotChosen, "reason": "chosen by hand: " + rel.Name})
	}
	if s.OnChosen != nil {
		s.OnChosen(ctx, took[0], rel.ID)
	}
	return ch, nil
}

func setTurnedDownSlotsTx(tx *sql.Tx, releaseID int64, ids []int64) error {
	if len(ids) == 0 {
		_, err := tx.Exec("UPDATE releases SET verdict = json_remove(verdict, '$.turned_down_slot_ids') WHERE id = ?", releaseID)
		return err
	}
	_, err := tx.Exec("UPDATE releases SET verdict = json_set(verdict, '$.turned_down_slot_ids', json(?)) WHERE id = ?", jsonText(ids), releaseID)
	return err
}
