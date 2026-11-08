package sources

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

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// dangerousWhy is the reason shown when a Release is refused because it once turned out to be a program.
const dangerousWhy = "its files were a program when it was downloaded before"

var (
	// Errors the choosing and list calls return, so callers can tell them apart.
	ErrNoRelease = errors.New("no such Release")
	ErrNoSlot    = errors.New("say which Slot: the Release matched none")
	ErrBusy      = errors.New("the Slot is busy")
	ErrState     = errors.New("the Release is not in that list")
)

// magnetBtih finds the info hash in a magnet link, as 40 hex characters or 32 base32 characters.
var magnetBtih = regexp.MustCompile(`(?i)[?&]xt=urn:btih:([0-9a-f]{40}|[a-z2-7]{32})(?:&|$)`)

// NeedsForceError says a Release failed the Pick rules, so choosing it needs force. Reasons are shown to the person.
type NeedsForceError struct{ Reasons []string }

func (e *NeedsForceError) Error() string {
	return "this Release failed the Pick rules (" + strings.Join(e.Reasons, "; ") + "); choose it with force to send it anyway"
}

// Choice is what choosing a Release by hand did: the Slots it went to, whether it was forced, and warnings to show.
type Choice struct {
	ReleaseID int64    `json:"release_id"`
	SlotID    int64    `json:"slot_id"`
	SlotIDs   []int64  `json:"slot_ids,omitempty"`
	Forced    bool     `json:"forced"`
	Warnings  []string `json:"warnings"`
}

// ListedRelease is a Release with the names a list shows beside it.
type ListedRelease struct {
	domain.Release
	SourceName string
	TitleID    int64
	TitleName  string
	SlotID     int64
	Version    string
}

// dangerousRels holds the info hashes and names of Releases that were once downloaded as a program.
type dangerousRels struct{ hashes, names map[string]bool }

// has reports whether a listing is a known program. The hash decides when there is one, since a name can be reused.
func (d dangerousRels) has(hash, name string) bool {
	if hash != "" {
		return d.hashes[strings.ToLower(hash)]
	}
	return name != "" && d.names[name]
}

// Choose picks a Release for a Slot by hand. With no slotID, a pack that matched several Slots goes to all that would gain, and otherwise to the first Slot it was not turned down for. A Release that failed the Pick rules, was turned down, or would not gain needs force, and the reasons come back in a NeedsForceError. Protected copies are never forced.
func (s *Searcher) Choose(ctx context.Context, releaseID, slotID int64, force bool, by domain.Actor) (Choice, error) {
	db := s.Store.Read()
	var rel domain.Release
	if err := database.Get(ctx, db, "releases", releaseID, &rel); err != nil {
		return Choice{}, ErrNoRelease
	}
	if slotID == 0 && len(rel.MatchedSlotIDs) > 1 {
		many, skipped, kind := s.packSlots(ctx, rel)
		if len(many) > 1 || len(many) == 1 && len(skipped) > 0 {
			return s.chooseMany(ctx, rel, many, skipped, kind == "artist", force, nil, by)
		}
		if len(many) == 0 && len(skipped) > 0 {

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

	// A music Slot that already has a copy as good is treated as refused, not silently replaced.
	if c.cur != nil && mediafiles.IsMusicRules(c.pr.Body) {
		jr := mediafiles.ReleaseFacts{Name: rel.Name, Parse: releasename.Parse(rel.Name, releasename.Hints{Size: rel.Size, Music: true}), Size: rel.Size, Protocol: rel.Protocol}
		if !mediafiles.Gains(jr, c.pr.Body, c.cur) {
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
	if c.sl.State == mediafiles.SlotNotOutYet {
		ch.Warnings = append(ch.Warnings, "the Release gate is skipped: this Title is not out yet")
	}
	if c.sl.State == mediafiles.SlotInLibrary {
		ch.Warnings = append(ch.Warnings, "this Version already has a good enough copy; the new one will replace it")
	}
	// The state is read again inside the write, so a copy protected or a Slot taken since the check above stops the pick.
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {

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
		if err := markChosenTx(tx, rel.ID, slotID, by); err != nil {
			return err
		}

		if only := TurnedDownSlots(rel); slices.Contains(only, slotID) {
			if err := setTurnedDownSlotsTx(tx, rel.ID, slices.DeleteFunc(slices.Clone(only), func(x int64) bool { return x == slotID })); err != nil {
				return err
			}
		}
		return moveSlotTx(tx, slotID, mediafiles.SlotChosen, "chosen by hand: "+rel.Name, by, rel.ID)
	})
	if err != nil {
		return Choice{}, err
	}
	data := map[string]any{"release_id": rel.ID, "forced": ch.Forced, "warnings": ch.Warnings}
	if len(overridden) > 0 {
		data["reasons_overridden"] = overridden
	}
	s.addEvent(ctx, by, nil, c.ti.ID, slotID, EventChosenByHand, "Chosen by hand: "+rel.Name, data)
	s.publish("slot", map[string]any{"id": slotID, "state": mediafiles.SlotChosen, "reason": "chosen by hand: " + rel.Name})
	if s.OnChosen != nil {
		s.OnChosen(ctx, slotID, rel.ID)
	}
	return ch, nil
}

// TurnDown rejects a Release for every Slot it matched.
func (s *Searcher) TurnDown(ctx context.Context, releaseID int64, reason string, by domain.Actor) error {
	return s.TurnDownFor(ctx, releaseID, 0, reason, by)
}

// TurnDownFor rejects a Release for one Slot, or for all when slotID is 0. Slots that were holding it go back to looking.
func (s *Searcher) TurnDownFor(ctx context.Context, releaseID, slotID int64, reason string, by domain.Actor) error {
	var rel domain.Release
	if err := database.Get(ctx, s.Store.Read(), "releases", releaseID, &rel); err != nil {
		return ErrNoRelease
	}
	if slotID > 0 && len(rel.MatchedSlotIDs) > 0 && !slices.Contains(rel.MatchedSlotIDs, slotID) {
		return ErrNoSlot
	}
	if reason == "" {
		reason = "turned down by hand"
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := markTurnedDownTx(tx, rel.ID, reason, by); err != nil {
			return err
		}
		if slotID > 0 {
			if err := setTurnedDownSlotsTx(tx, rel.ID, append(TurnedDownSlots(rel), slotID)); err != nil {
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

// RemoveHeld clears one Held Release (id above 0) or all of them, and returns how many. A Release that is not held returns ErrState.
func (s *Searcher) RemoveHeld(ctx context.Context, id int64, by domain.Actor) (int, error) {
	n := 0
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		ids, err := heldReleaseIDsTx(tx, id)
		if err != nil {
			return err
		}
		if id > 0 && len(ids) == 0 {
			return ErrState
		}
		for _, rid := range ids {
			if err := markRemovedByHandTx(tx, rid, by); err != nil {
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

// RemoveTurnedDown takes one Release (id above 0) or all off the Turned down list and returns how many. A Release with a download or send attempt is kept as passed over or refused, so its history stays. The others are deleted.
func (s *Searcher) RemoveTurnedDown(ctx context.Context, id int64, by domain.Actor) (int, error) {
	n := 0
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		removed, err := removeTurnedDownTx(tx, id, by)
		if err != nil {
			return err
		}
		n += removed
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

// List returns Releases in a state, newest first (held ones by wait end), with the total before paging.
func (s *Searcher) List(ctx context.Context, state string, limit, offset int) ([]ListedRelease, int, error) {
	db := s.Store.Read()
	rels, total, err := listReleases(ctx, db, state, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ListedRelease, 0, len(rels))
	for _, r := range rels {
		l := ListedRelease{Release: r}
		l.SourceName = sourceName(ctx, db, r.SourceID)
		if only := TurnedDownSlots(r); state == "turned_down" && len(only) > 0 {
			l.SlotID = only[0]
		} else if len(r.MatchedSlotIDs) > 0 {
			l.SlotID = r.MatchedSlotIDs[0]
			l.TitleID, l.TitleName, l.Version, _ = mediafiles.SlotNames(ctx, db, l.SlotID)
		}
		out = append(out, l)
	}
	return out, total, nil
}

// TurnedDownSlots returns the Slots a Release was turned down for. An empty list means it was turned down whole.
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

// RefusedByRules reports whether a Release was refused for a reason a person can override, not only for being turned down.
func RefusedByRules(rel domain.Release) bool {
	if rel.State == "refused" || rel.Verdict["dangerous_files"] == true {
		return true
	}
	if o, _ := rel.Verdict["outcome"].(string); o != "refused" {
		return false
	}
	reasons, _ := rel.Verdict["reasons"].([]any)
	if len(reasons) == 0 {
		return true
	}
	for _, x := range reasons {
		r, _ := x.(map[string]any)
		if r["effect"] == "fail" && !(r["step"] == "hard" && r["rule"] == "turned_down") {
			return true
		}
	}
	return false
}

// failReasons lists in plain words why a Release did not pass, never empty.
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

// canTakePick reports whether a Slot in this state can be given a Release now.
func canTakePick(st mediafiles.SlotState) bool {
	return !busySlot(st) && st != mediafiles.SlotRemoved &&
		(mediafiles.Transition(st, mediafiles.SlotChosen) == nil || mediafiles.Transition(st, mediafiles.SlotSearching) == nil)
}

// busySlot reports whether a Slot already has work in flight.
func busySlot(st mediafiles.SlotState) bool {
	return st == mediafiles.SlotChosen || st == mediafiles.SlotDownloading || st == mediafiles.SlotImporting || st == mediafiles.SlotSearching
}

// unholdTx sends every Slot that was holding the Release back to looking. A Slot with a file goes to better wanted so the file is kept.
func unholdTx(tx *sql.Tx, releaseID int64, reason string, by domain.Actor) error {
	list, err := mediafiles.SlotsHeldBy(tx, releaseID)
	if err != nil {
		return err
	}
	for _, h := range list {
		to := mediafiles.SlotWanted
		if h.HasFile {
			to = mediafiles.SlotBetterWanted
		}
		if err := mediafiles.ClearSlotHeldTx(tx, h.ID); err != nil {
			return err
		}
		if err := mediafiles.MoveSlotTx(tx, h.ID, to, reason, by); err != nil {
			return err
		}
	}
	return nil
}

// turnedDownFor reports whether the Release was turned down for this Slot.
func turnedDownFor(rel domain.Release, slotID int64) bool {
	if only := TurnedDownSlots(rel); len(only) > 0 {
		return slices.Contains(only, slotID)
	}
	return rel.State == "turned_down"
}

// packSlots returns the matched Slots of a show or album Release that could take it, and why each other Slot was left out. Other kinds return none.
func (s *Searcher) packSlots(ctx context.Context, rel domain.Release) (out []int64, skipped []string, kind string) {
	kind, _ = mediafiles.SlotKind(ctx, s.Store.Read(), rel.MatchedSlotIDs[0])
	if kind != "show" && kind != "artist" {
		return nil, nil, kind
	}
	jr := mediafiles.ReleaseFacts{Name: rel.Name, Parse: releasename.Parse(rel.Name, releasename.Hints{Size: rel.Size, Show: kind == "show", Music: kind == "artist"}), Size: rel.Size, Protocol: rel.Protocol}
	for _, id := range rel.MatchedSlotIDs {
		st, err := mediafiles.SlotStateOf(ctx, s.Store.Read(), id)
		if err != nil || !canTakePick(mediafiles.SlotState(st)) || turnedDownFor(rel, id) {
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
		if c, err := s.loadSlot(ctx, id); err == nil && c.cur != nil && !mediafiles.Gains(jr, c.pr.Body, c.cur) {
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

// listingHash returns the lower-case hex info hash of a listing, from the field or the magnet link, or "".
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

// dangerousRel reports whether the Release is known to be a program, by its hash or name or by a failed download of it.
func (s *Searcher) dangerousRel(ctx context.Context, rel domain.Release) bool {
	h := ""
	if rel.InfoHash != nil {
		h = *rel.InfoHash
	}
	if d := s.dangerousSeen(ctx); d.has(listingHash(h, rel.LinkRef), rel.Name) {
		return true
	}
	return dangerousDownloadOf(ctx, s.Store.Read(), rel.ID)
}

// dangerousSeen collects every hash and name whose download failed for dangerous files.
func (s *Searcher) dangerousSeen(ctx context.Context) dangerousRels {
	d := dangerousRels{hashes: map[string]bool{}, names: map[string]bool{}}
	pairs, err := dangerousDownloads(ctx, s.Store.Read())
	if err != nil {
		return d
	}
	for _, p := range pairs {
		if releasename.ValidInfoHash(p.hash) {
			d.hashes[p.hash] = true
		}
		if p.name != "" {
			d.names[p.name] = true
		}
	}
	return d
}

// unprotectedSlots returns the matched Slots that can take a pick and have no protected copy.
func (s *Searcher) unprotectedSlots(ctx context.Context, rel domain.Release) []int64 {
	var out []int64
	for _, id := range rel.MatchedSlotIDs {
		st, err := mediafiles.SlotStateOf(ctx, s.Store.Read(), id)
		if err != nil || !canTakePick(mediafiles.SlotState(st)) || turnedDownFor(rel, id) || slotHasProtectedCopy(ctx, s.Store.Read(), id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// episodeLabel names an episode as S01E02 for messages.
func episodeLabel(it mediafiles.Item) string {
	if it.Numbers.Season == nil || it.Numbers.Episode == nil {
		return "an episode"
	}
	return fmt.Sprintf("S%02dE%02d", *it.Numbers.Season, *it.Numbers.Episode)
}

// chooseMany picks one Release for several Slots at once. Slots that cannot take it are passed over, and it fails only when none can. An album with left-out tracks needs force because the copies would mix.
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
				continue
			}
			if st == mediafiles.SlotNotOutYet && !slices.Contains(ch.Warnings, notAired) {
				ch.Warnings = append(ch.Warnings, notAired)
			}
			if err := passOverHeldTx(tx, id, rel.ID, by); err != nil {
				return err
			}
			if err := moveSlotTx(tx, id, mediafiles.SlotChosen, "chosen by hand: "+rel.Name, by, rel.ID); err != nil {
				return err
			}
			took = append(took, id)
		}
		if len(took) == 0 {
			return busyErr(mediafiles.SlotChosen)
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
	titleID, _ := mediafiles.SlotTitleID(ctx, s.Store.Read(), took[0])
	for _, id := range took {
		s.addEvent(ctx, by, nil, titleID, id, EventChosenByHand, "Chosen by hand: "+rel.Name, data)
		s.publish("slot", map[string]any{"id": id, "state": mediafiles.SlotChosen, "reason": "chosen by hand: " + rel.Name})
	}
	if s.OnChosen != nil {
		s.OnChosen(ctx, took[0], rel.ID)
	}
	return ch, nil
}
