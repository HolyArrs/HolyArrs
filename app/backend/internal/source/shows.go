package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/judge"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediainfo"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
	"github.com/HolyArrs/holyarrs/app/backend/internal/tagset"
)

// KindSearchSeason is the job kind that searches one season of a show for one Version.
const KindSearchSeason = "search_season"

const (
	// Show search settings.
	SettingMissingShows = "missing_search_shows"
	SettingPackUpgrade  = "season_pack_upgrade"
	SettingPackPrefer   = "season_pack_prefer"
	SettingMultiSeason  = "multi_season_packs"
)

// showRules are the season pack settings read once per search.
type showRules struct {
	packAny     bool
	packsFirst  bool
	multiSeason bool
}

func (s *Searcher) showRules(ctx context.Context) showRules {
	db := s.Store.Read()
	return showRules{
		packAny:     Setting(ctx, db, SettingPackUpgrade) == "any",
		packsFirst:  Setting(ctx, db, SettingPackPrefer) == "always",
		multiSeason: Setting(ctx, db, SettingMultiSeason) == "true",
	}
}

// epSlot is one episode of a show with its Slot in one Version.
type epSlot struct {
	ref judge.EpisodeRef
	it  domain.Item
	// sl is nil when the episode has no Slot in this Version.
	sl  *domain.Slot
	cur *judge.Current
	// kept means the episode, its season, the Version and the show are all kept up to date.
	kept bool
	// soon means aired, or airing within a day, when season packs may be taken.
	soon    bool
	runtime int
}

// showSet is a show search: the show's episodes in one Version and the Slots asked for.
type showSet struct {
	ti     domain.Title
	ver    domain.Version
	pr     domain.PickRules
	eps    []epSlot
	target map[int64]bool
	// notOut holds target Slots that were not out yet; a hand search skips that gate.
	notOut map[int64]bool
	// single is an episode search, where season packs are refused.
	single bool
}

// EpisodeLabel names an episode item: S01E02, or its air day for a daily show.
func EpisodeLabel(it domain.Item) string {
	s, e := 0, 0
	if it.Numbers.Season != nil {
		s = *it.Numbers.Season
	}
	if it.Numbers.Episode != nil {
		e = *it.Numbers.Episode
	}
	return fmt.Sprintf("S%02dE%02d", s, e)
}

// currentOf describes the file a Slot holds for the better-than-what-we-have
// step, or nil when it holds none.
func currentOf(ctx context.Context, db *sql.DB, sl domain.Slot, pr domain.PickRules) *judge.Current {
	if sl.FileID == nil {
		return nil
	}
	var f domain.File
	if err := database.Get(ctx, db, "files", *sl.FileID, &f); err != nil || f.State != "present" {
		return nil
	}
	name := strings.TrimSuffix(path.Base(f.RelativePath), path.Ext(f.RelativePath))
	if f.ReleaseName != nil && *f.ReleaseName != "" {
		name = *f.ReleaseName
	}
	if cur := judge.CurrentOf(name, f.AddedAt, pr.Body); cur.Step != "" {
		return &cur
	}
	if domain.IsMusicRules(pr.Body) {
		if word := trackFormat(ctx, db, f); word != "" {
			if cur := judge.CurrentOf(name+" ["+word+"]", f.AddedAt, pr.Body); cur.Step != "" {
				return &cur
			}
		}
	}
	return nil
}

// trackFormat is the format word of a track file: its extension, or for a file
// that may hold either kind the codec read from its library folder. A lossless
// copy must stay protected however it was named.
func trackFormat(ctx context.Context, db *sql.DB, f domain.File) string {
	ext := strings.ToLower(path.Ext(f.RelativePath))
	switch ext {
	case "":
		return ""
	case ".m4a", ".mka", ".mp4":
		var lib string
		if db.QueryRowContext(ctx, "SELECT lf.path FROM versions v JOIN library_folders lf ON lf.id = v.library_folder_id WHERE v.id = ?", f.VersionID).Scan(&lib) != nil {
			return ""
		}
		info, err := mediainfo.Inspect(filepath.Join(lib, filepath.FromSlash(f.RelativePath)))
		if err != nil {
			return ""
		}
		if l, known := info.Lossless(); known && l {
			return "LOSSLESS"
		}
		return info.AudioWord()
	case ".tak":
		return "LOSSLESS"
	}
	return strings.ToUpper(strings.TrimPrefix(ext, "."))
}

// loadShow reads a show's episodes with their Slots in one Version.
func (s *Searcher) loadShow(ctx context.Context, ti domain.Title, ver domain.Version) (showSet, error) {
	db := s.Store.Read()
	set := showSet{ti: ti, ver: ver, target: map[int64]bool{}, notOut: map[int64]bool{}}
	if err := database.Get(ctx, db, "pick_rules", ver.PickRulesID, &set.pr); err != nil {
		return set, err
	}
	parts, err := database.Select[domain.Part](ctx, db, "parts", "title_id = ?", ti.ID)
	if err != nil {
		return set, err
	}
	partKept := map[int64]bool{}
	for _, p := range parts {
		partKept[p.ID] = p.KeepUpToDate && p.DroppedAt == nil
	}
	its, err := database.Select[domain.Item](ctx, db, "items", `title_id = ? AND kind = 'episode' AND dropped_at IS NULL
		ORDER BY json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')`, ti.ID)
	if err != nil {
		return set, err
	}
	slots, err := database.Select[domain.Slot](ctx, db, "slots", "version_id = ? AND item_id IN (SELECT id FROM items WHERE title_id = ?)", ver.ID, ti.ID)
	if err != nil {
		return set, err
	}
	byItem := map[int64]*domain.Slot{}
	for i := range slots {
		byItem[slots[i].ItemID] = &slots[i]
	}
	soon := time.Now().Add(24 * time.Hour)
	for _, it := range its {
		e := epSlot{it: it, sl: byItem[it.ID]}
		e.ref = judge.EpisodeRef{ItemID: it.ID}
		if it.Numbers.Season != nil {
			e.ref.Season = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			e.ref.Episode = *it.Numbers.Episode
		}
		if it.Numbers.Absolute != nil {
			e.ref.Absolute = *it.Numbers.Absolute
		}
		if it.AirDay != nil {
			e.ref.AirDay = *it.AirDay
		}
		if it.Runtime != nil {
			e.runtime = *it.Runtime
		}
		e.soon = it.AirDate != nil && !it.AirDate.After(soon)
		e.kept = it.KeepUpToDate && ver.KeepUpToDate && ti.KeepUpToDate && (it.PartID == nil || partKept[*it.PartID])
		if e.sl != nil {
			e.cur = currentOf(ctx, db, *e.sl, set.pr)
		}
		set.eps = append(set.eps, e)
	}
	return set, nil
}

func (set showSet) ref() judge.TitleRef {
	r := judge.TitleRef{ID: set.ti.ID, Name: set.ti.Name, Aliases: set.ti.Aliases, Kind: "show"}
	if set.ti.Year != nil {
		r.Year = *set.ti.Year
	}
	if set.ti.OriginalLanguage != nil {
		r.OriginalLanguage = *set.ti.OriginalLanguage
	}
	return r
}

// labels names each episode Slot, like S01E02.
func (set showSet) labels() map[int64]string {
	out := map[int64]string{}
	for _, e := range set.eps {
		if e.sl != nil {
			out[e.sl.ID] = e.ref.Label()
		}
	}
	return out
}

func (set showSet) order() string {
	if set.ti.EpisodeOrder != nil {
		return *set.ti.EpisodeOrder
	}
	return "aired"
}

// coverLabel names the episodes a release holds in a few words.
func coverLabel(eps []*epSlot) string {
	if len(eps) == 0 {
		return "no episode"
	}
	if len(eps) > 3 {
		seasons := []string{}
		for _, e := range eps {
			n := fmt.Sprintf("%d", e.ref.Season)
			if !slices.Contains(seasons, n) {
				seasons = append(seasons, n)
			}
		}
		return fmt.Sprintf("%d episodes of season %s", len(eps), strings.Join(seasons, ", "))
	}
	ls := make([]string, len(eps))
	for i, e := range eps {
		ls[i] = e.ref.Label()
	}
	return strings.Join(ls, ", ")
}

var lookingNow = []domain.SlotState{domain.SlotWanted, domain.SlotBetterWanted}

// judgeShow judges every release for a show search and ranks them.
func (s *Searcher) judgeShow(ctx context.Context, set showSet, all []found, hand, skipGate bool, rules showRules) ([]judge.Release, []judge.Verdict, error) {
	bad := s.dangerousSeen(ctx)
	refs := make([]judge.EpisodeRef, len(set.eps))
	byItem := map[int64]*epSlot{}
	for i := range set.eps {
		refs[i] = set.eps[i].ref
		byItem[set.eps[i].it.ID] = &set.eps[i]
	}
	turned := map[int64][]string{}
	turnedFor := func(slotID int64) []string {
		if l, ok := turned[slotID]; ok {
			return l
		}
		l, _ := s.turnedDown(ctx, slotID)
		turned[slotID] = l
		return l
	}
	tref := set.ref()
	checks := Checks(ctx, s.Store.Read())
	rels := make([]judge.Release, len(all))
	vs := make([]judge.Verdict, len(all))
	for i, f := range all {
		p := releasename.Parse(f.item.Title, releasename.Hints{Size: f.item.Size, Show: true})
		rels[i] = judge.Release{Name: f.item.Title, Parse: p, Size: f.item.Size, Protocol: f.item.Protocol, Seeders: f.item.Seeders,
			PublishedAt: f.item.Published, InfoHash: f.item.InfoHash, SourcePriority: f.src.Priority, Dangerous: bad.has(listingHash(f.item.InfoHash, f.item.Link), f.item.Title)}
		ss := judge.SlotSet{Match: judge.MatchTitle(p, []judge.TitleRef{tref}), MinSeeders: f.src.MinSeeders, HandSearch: hand, SkipGate: skipGate, Out: true,
			Checks: checks}
		if ss.Match.Title != nil {
			set.lines(&ss, rels[i], refs, byItem, rules, turnedFor)
		}
		vs[i] = judge.Judge(rels[i], ss, set.pr.Body)
	}
	if rules.packsFirst {
		judge.RankPacksFirst(rels, vs, set.pr.Body)
	} else {
		judge.Rank(rels, vs, set.pr.Body)
	}
	return rels, vs, nil
}

func matchFail(rule, detail string) judge.Reason {
	return judge.Reason{Step: "match", Rule: rule, Effect: "fail", Detail: detail}
}

// lines works out which episodes and Slots a show release covers and applies
// the season pack rules.
func (set showSet) lines(ss *judge.SlotSet, rel judge.Release, refs []judge.EpisodeRef, byItem map[int64]*epSlot, rules showRules,
	turnedFor func(int64) []string) {
	p := rel.Parse
	found, why := judge.MatchEpisodes(p, set.order(), refs)
	if why != "" {
		ss.MatchReasons = append(ss.MatchReasons, matchFail("episode", why))
		return
	}
	covered := make([]*epSlot, 0, len(found))
	for _, r := range found {
		covered = append(covered, byItem[r.ItemID])
	}
	ss.MatchReasons = append(ss.MatchReasons, judge.Reason{Step: "match", Rule: "episode", Effect: "pass", Detail: "holds " + coverLabel(covered)})
	pack := judge.IsPack(p)
	if (p.Kind.Value == releasename.KindMultiSeason || p.Kind.Value == releasename.KindComplete) && !rules.multiSeason {
		ss.MatchReasons = append(ss.MatchReasons, matchFail("several_seasons", "holds more than one season; Releases with several seasons are not taken (Settings > Search)"))
		return
	}
	if set.single && pack {
		ss.MatchReasons = append(ss.MatchReasons, matchFail("season_pack", "a season pack; an episode search takes only that episode"))
		return
	}
	asked := false
	for _, e := range covered {
		asked = asked || e.sl != nil && set.target[e.sl.ID]
	}
	if !asked {
		ss.MatchReasons = append(ss.MatchReasons, matchFail("not_asked", "holds "+coverLabel(covered)+", not what was searched for"))
		return
	}
	runtime := 0
	for _, e := range covered {
		runtime += e.runtime
	}
	ss.RuntimeMinutes = runtime
	// No length known: count the episodes instead.
	if runtime == 0 {
		ss.FloorEpisodes = len(covered)
	}
	for _, e := range covered {
		if a := e.it.AirDate; a != nil && (ss.OutSince == nil || a.After(*ss.OutSince)) {
			ss.OutSince = a
		}
	}

	if len(covered) == 1 {
		e := covered[0]
		ss.SlotIDs = []int64{e.sl.ID}
		ss.Current = e.cur
		ss.TurnedDown = turnedFor(e.sl.ID)
		ss.Out = !set.notOut[e.sl.ID]
		if ss.Out {
			ss.GateDetail = e.ref.Label() + " has aired"
		} else {
			ss.GateDetail = e.ref.Label() + " has not aired yet"
		}
		return
	}

	if pack {
		for _, e := range covered {
			if e.kept && !e.soon {
				ss.Out = false
				ss.GateDetail = fmt.Sprintf("not every episode of season %d has aired (%s has not)", e.ref.Season, e.ref.Label())
				break
			}
		}
		if ss.Out {
			ss.GateDetail = "every episode it holds has aired"
		}
	}
	var noGain, turnedSome []string
	targetTurned := false
	for _, e := range covered {
		if e.sl == nil {
			continue
		}
		isTarget := set.target[e.sl.ID]
		if !isTarget && !(e.kept && slices.Contains(lookingNow, e.sl.State)) {
			if e.kept && e.sl.State != domain.SlotNotOutYet {
				noGain = append(noGain, e.ref.Label()+" is "+strings.ReplaceAll(string(e.sl.State), "_", " "))
			}
			continue
		}
		if l := turnedFor(e.sl.ID); slices.Contains(l, rel.Name) || rel.InfoHash != "" && slices.Contains(l, rel.InfoHash) {
			turnedSome = append(turnedSome, e.ref.Label())
			targetTurned = targetTurned || isTarget
			continue
		}
		if !judge.Gains(rel, set.pr.Body, e.cur) {
			noGain = append(noGain, e.ref.Label()+" already has a copy as good")
			continue
		}
		if isTarget && set.notOut[e.sl.ID] {
			ss.Out = false
			ss.GateDetail = e.ref.Label() + " has not aired yet"
		}
		ss.SlotIDs = append(ss.SlotIDs, e.sl.ID)
	}
	if len(turnedSome) > 0 {
		ss.MatchReasons = append(ss.MatchReasons, judge.Reason{Step: "match", Rule: "turned_down_some", Effect: "warn",
			Detail: "turned down before for " + strings.Join(turnedSome, ", ") + "; not used for those"})
	}
	if len(noGain) > 0 {
		r := judge.Reason{Step: "match", Rule: "pack_share", Effect: "warn", Detail: "not every episode would gain: " + strings.Join(noGain, "; ")}
		if !rules.packAny {
			r.Effect = "fail"
			r.Detail += " (Settings > Search: every episode must gain)"
		}
		ss.MatchReasons = append(ss.MatchReasons, r)
	}
	hasTarget := false
	for _, id := range ss.SlotIDs {
		hasTarget = hasTarget || set.target[id]
	}
	switch {
	case !hasTarget && targetTurned && len(ss.SlotIDs) == 0:
		ss.TurnedDown = []string{rel.Name}
	case !hasTarget:
		ss.MatchReasons = append(ss.MatchReasons, matchFail("no_gain", "not better than the copies in place of what was searched for"))
	}
}

// showPick is one release chosen or held for some Slots of a show search.
type showPick struct {
	idx   int
	slots []int64
	held  bool
}

// pickShow takes the passing releases best first. A release holding an episode
// already taken in this pass is skipped, so each Slot gets one pick.
func (s *Searcher) pickShow(ctx context.Context, all []found, vs []judge.Verdict, now time.Time) []showPick {
	order := []int{}
	for i := range vs {
		if vs[i].Passed && vs[i].Rank > 0 && len(vs[i].SlotIDs) > 0 {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return vs[order[a]].Rank < vs[order[b]].Rank })
	taken := map[int64]bool{}
	var out []showPick
	for _, i := range order {
		clash := false
		for _, id := range vs[i].SlotIDs {
			clash = clash || taken[id]
		}
		if clash {
			continue
		}
		if over, why := s.overGrabLimit(ctx, all[i].src, now); over {
			vs[i].Outcome = "passed_over"
			vs[i].Reasons = append(vs[i].Reasons, judge.Reason{Step: "rank", Rule: "source_limit", Effect: "rank", Detail: why})
			continue
		}
		held := vs[i].HoldUntil != nil && now.Before(*vs[i].HoldUntil)
		vs[i].Outcome = "chosen"
		if held {
			vs[i].Outcome = "held"
		}
		for _, id := range vs[i].SlotIDs {
			taken[id] = true
		}
		out = append(out, showPick{idx: i, slots: vs[i].SlotIDs, held: held})
	}
	return out
}

// settlePicks settles show picks, choosing or holding each release for all its
// Slots in one write. It returns how many were chosen and held, and the Slots
// they took.
func (s *Searcher) settlePicks(ctx context.Context, by domain.Actor, jobID *int64, titleID int64, labels map[int64]string, all []found, vs []judge.Verdict, ids []int64,
	picks []showPick, how string) (int, int, map[int64]bool) {
	chosen, held := 0, 0
	took := map[int64]bool{}
	for _, pk := range picks {
		name := all[pk.idx].item.Title
		var err error
		if pk.held {
			err = s.settleHeldMany(ctx, pk.slots, ids[pk.idx], name, *vs[pk.idx].HoldUntil, by)
		} else {
			err = s.settleChosenMany(ctx, pk.slots, ids[pk.idx], name, by)
		}
		if errors.Is(err, ErrBusy) || err != nil {
			continue
		}
		eps := make([]string, 0, len(pk.slots))
		for _, id := range pk.slots {
			took[id] = true
			if l := labels[id]; l != "" && !slices.Contains(eps, l) {
				eps = append(eps, l)
			}
		}
		data := map[string]any{"release_id": ids[pk.idx], "slot_ids": pk.slots, "episodes": eps}
		for _, id := range pk.slots {
			if pk.held {
				s.addEvent(ctx, by, jobID, titleID, id, EventReleaseHeld, fmt.Sprintf("%s holding %s for %s", how, name, strings.Join(eps, ", ")), data)
				s.publish("slot", map[string]any{"id": id, "state": domain.SlotHeld, "reason": "holding " + name})
			} else {
				s.addEvent(ctx, by, jobID, titleID, id, EventReleaseChosen, fmt.Sprintf("%s chose %s for %s", how, name, strings.Join(eps, ", ")), data)
				s.publish("slot", map[string]any{"id": id, "state": domain.SlotChosen, "reason": "chose " + name})
			}
		}
		if pk.held {
			held++
			continue
		}
		chosen++
		if s.OnChosen != nil {
			s.OnChosen(ctx, pk.slots[0], ids[pk.idx])
		}
	}
	return chosen, held, took
}

// settleChosenMany makes a release the pick of several Slots in one write, as
// for a season pack: one release, one Download, many Slots. Every Slot is read
// again inside the write, and when one is no longer looking it returns ErrBusy
// and nothing changes.
func (s *Searcher) settleChosenMany(ctx context.Context, slotIDs []int64, releaseID int64, name string, by domain.Actor) error {
	if len(slotIDs) == 0 {
		return errors.New("no Slot to choose the Release for")
	}
	if len(slotIDs) == 1 {
		return s.settleChosen(ctx, slotIDs[0], releaseID, name, by)
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range slotIDs {
			from, err := choiceStateTx(tx, id)
			if err != nil {
				return err
			}
			if !slices.Contains(pickStates, from) {
				return busyErr(from)
			}
		}
		for _, id := range slotIDs {
			if err := moveSlotTx(tx, id, domain.SlotChosen, "chose "+name, by, releaseID); err != nil {
				return err
			}
			if err := passOverHeldTx(tx, id, releaseID, by); err != nil {
				return err
			}
		}
		return pickReleaseTx(tx, releaseID, "chosen", nil, slotIDs, by)
	})
	if errors.Is(err, ErrBusy) {
		s.lostRace(ctx, releaseID, by)
	}
	return err
}

// settleHeldMany holds a release for several Slots until its wait is over.
func (s *Searcher) settleHeldMany(ctx context.Context, slotIDs []int64, releaseID int64, name string, until time.Time, by domain.Actor) error {
	if len(slotIDs) == 1 {
		return s.settleHeld(ctx, slotIDs[0], releaseID, name, until, by)
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range slotIDs {
			from, err := choiceStateTx(tx, id)
			if err != nil {
				return err
			}
			if !slices.Contains(pickStates, from) {
				return busyErr(from)
			}
		}
		for _, id := range slotIDs {
			if err := passOverHeldTx(tx, id, releaseID, by); err != nil {
				return err
			}
			if err := moveSlotTx(tx, id, domain.SlotHeld, "holding "+name, by, releaseID); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE slots SET held_release_id = ?, next_try_at = ? WHERE id = ?", releaseID, database.FormatTime(until), id); err != nil {
				return err
			}
		}
		return pickReleaseTx(tx, releaseID, "held", &until, slotIDs, by)
	})
	if errors.Is(err, ErrBusy) {
		s.lostRace(ctx, releaseID, by)
	}
	return err
}

// pickReleaseTx marks a release chosen or held for its Slots. The Slots it
// fills are added to the verdict's pick_slot_ids, which the Download reads;
// Slots of another Version picked for the same release stay listed, and the
// Download keeps to one Version and the Slots still chosen for it.
func pickReleaseTx(tx *sql.Tx, releaseID int64, state string, until *time.Time, slotIDs []int64, by domain.Actor) error {
	_, err := tx.Exec(`UPDATE releases SET state = ?, hold_until = ?,
		verdict = json_set(COALESCE(verdict, '{}'), '$.pick_slot_ids', (SELECT json_group_array(value) FROM (
			SELECT value FROM json_each(COALESCE(json_extract(verdict, '$.pick_slot_ids'), '[]')) UNION SELECT value FROM json_each(?)))),
		matched_slot_ids = (SELECT json_group_array(value) FROM (SELECT value FROM json_each(matched_slot_ids) UNION SELECT value FROM json_each(?))),
		updated_at = ?, updated_by = ? WHERE id = ?`,
		state, timeOrNil(until), jsonText(slotIDs), jsonText(slotIDs), database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

// PickSlots lists the Slots a release was chosen or held for when it fills
// more than one, or nil otherwise.
func PickSlots(rel domain.Release) []int64 {
	xs, _ := rel.Verdict["pick_slot_ids"].([]any)
	if len(xs) < 2 {
		return nil
	}
	out := make([]int64, 0, len(xs))
	for _, x := range xs {
		if f, ok := x.(float64); ok {
			out = append(out, int64(f))
		}
	}
	return out
}

// showQuery is the search for one episode (daily: its air day; absolute: its
// running number) or, with ep nil, one season.
func showQuery(ti domain.Title, season int, ep *epSlot) Query {
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
	case order == "absolute" && ep.ref.Absolute > 0:
		q.Season, q.Episode = 0, strconv.Itoa(ep.ref.Absolute)
		q.Words = fmt.Sprintf("%02d", ep.ref.Absolute)
	default:
		q.Season, q.Episode = ep.ref.Season, strconv.Itoa(ep.ref.Episode)
		q.Words = ep.ref.Label()
	}
	return q
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
	var part domain.Part
	if err := database.Get(ctx, db, "parts", r.Job.Subject.ID, &part); err != nil {
		return fmt.Errorf("season %d: %w", r.Job.Subject.ID, err)
	}
	var ti domain.Title
	if err := database.Get(ctx, db, "titles", part.TitleID, &ti); err != nil {
		return err
	}
	vid, _ := r.Job.Params["version_id"].(float64)
	var ver domain.Version
	if err := database.Get(ctx, db, "versions", int64(vid), &ver); err != nil || ver.TitleID != ti.ID {
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
		ok := show || slices.Contains([]domain.SlotState{domain.SlotWanted, domain.SlotBetterWanted, domain.SlotFailed, domain.SlotRejected, domain.SlotStalled}, e.sl.State)
		if ok && (e.kept || anyEp || show) {
			looking = append(looking, e)
		}
		missing := e.sl.State == domain.SlotWanted || e.cur == nil && (e.sl.State == domain.SlotFailed || e.sl.State == domain.SlotRejected)
		if e.kept && (!missing || !e.soon) {
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
			to := domain.SlotWanted
			if e.cur != nil || e.sl.State == domain.SlotBetterWanted {
				to = domain.SlotBetterWanted
			}
			if st := s.slotState(ctx, e.sl.ID); st == domain.SlotSearching {
				_ = store.MoveSlot(ctx, s.Store, e.sl.ID, to, reason, by)
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
			if err := store.MoveSlot(ctx, s.Store, e.sl.ID, domain.SlotSearching, "searching season "+strconv.Itoa(season), by); err != nil {
				delete(set.target, e.sl.ID)
				continue
			}
			s.publish("slot", map[string]any{"id": e.sl.ID, "state": domain.SlotSearching})
		}
	}

	found, failures, answered, skipped, err := s.askAll(ctx, r, showQuery(ti, season, nil), hand, true, ti.Tags)
	if err != nil {
		return err
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
			if _, err := tx.Exec("UPDATE items SET last_searched_at = ? WHERE id = ?", database.FormatTime(now), e.it.ID); err != nil {
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

func (s *Searcher) slotState(ctx context.Context, id int64) domain.SlotState {
	var st string
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT state FROM slots WHERE id = ?", id).Scan(&st)
	return domain.SlotState(st)
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
		_, created, err := s.Queue.EnqueueAt(ctx, KindSearchSlot, &domain.Subject{Type: "slot", ID: e.sl.ID}, params, key, by, start.Add(time.Duration(n)*bulkStagger))
		if err != nil {
			return n, err
		}
		if created {
			n++
		}
	}
	return n, nil
}

// askAll asks every Source used for this kind of search; tv picks the Sources
// and categories for shows. A Source with tags is asked only for a title that
// shares one of them.
func (s *Searcher) askAll(ctx context.Context, r *jobs.Run, q Query, hand, tv bool, titleTags []string) ([]found, []string, int, []skippedSource, error) {
	now := time.Now().UTC()
	srcs, err := database.Select[domain.Source](ctx, s.Store.Read(), "sources",
		"state IN ('enabled', 'paused') ORDER BY priority, id")
	if err != nil {
		return nil, nil, 0, nil, err
	}
	var all []found
	var failures []string
	var skipped []skippedSource
	answered := 0
	for i, src := range srcs {
		if src.Format == "rss" || hand && !src.UseFor.HandSearch || !hand && !src.UseFor.AutoSearch || tv && !handlesTV(src) || q.Music && !handlesMusic(src) {
			continue
		}
		if !tagset.SourceServes(src.Tags, titleTags) {
			skipped = append(skipped, skippedSource{ID: src.ID, Name: src.Name, Why: tagsWhy + strings.Join(src.Tags, ", ") + ") are not on this title"})
			continue
		}
		if src.State == "paused" && (src.PausedUntil == nil || src.PausedUntil.After(now)) {
			why := "paused"
			if src.PauseReason != nil && *src.PauseReason != "" {
				why = *src.PauseReason
			}
			if b := BlockOf(src); b != "" {
				why = b
			}
			skipped = append(skipped, skippedSource{ID: src.ID, Name: src.Name, Why: why})
			continue
		}
		_ = r.Progress(ctx, float64(i)/float64(len(srcs)+1), "searching "+src.Name)
		if s.onAsk != nil {
			s.onAsk(src.Name)
		}
		qq := q
		if q.Music {
			qq.Categories = musicCategories(src)
		} else if tv {
			qq.Categories = tvCategories(src)
		} else {
			qq.Categories = src.Categories["movie"]
		}
		cl, err := s.client(ctx, src)
		var items []Item
		if err == nil {
			items, err = cl.Search(ctx, qq)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, 0, nil, ctx.Err()
			}
			failures = append(failures, src.Name+": "+err.Error())
			skipped = append(skipped, skippedSource{ID: src.ID, Name: src.Name, Why: failWhy(err)})
			s.pause(ctx, src, err)
			continue
		}
		answered++
		s.recovered(ctx, src)
		for _, item := range items {
			all = append(all, found{src: src, item: item, keys: secretValues(cl.cfg)})
		}
	}
	return all, failures, answered, skipped, nil
}

// skippedSource is a Source a search could not use: paused, or failed this
// time. Why is the pause reason or the kind of failure.
type skippedSource struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Why  string `json:"why"`
}

// failWhy names the kind of a Source failure in the pause reason words.
func failWhy(err error) string {
	var se *Error
	// A site's bot check or ban.
	//
	if errors.As(err, &se) && se.Detail != "" {
		return se.Detail
	}
	if errors.As(err, &se) && se.Kind != "" {
		return se.Kind
	}
	return KindDown
}

// noAnswer is the search_failed summary and data when no Source answered. ok
// is false when a Source answered or none was tried. failed is false when
// every Source was only paused: the search is written down but is not a
// failed job.
func noAnswer(failures []string, answered int, skipped []skippedSource) (reason string, data map[string]any, ok, failed bool) {
	if answered > 0 || len(skipped) == 0 {
		return "", nil, false, false
	}
	failed = len(failures) > 0
	tagsOnly := !failed
	if !failed {
		for _, k := range skipped {
			failures = append(failures, k.Name+": "+pauseWords(k.Why))
			tagsOnly = tagsOnly && strings.HasPrefix(k.Why, tagsWhy)
		}
	}
	if tagsOnly {
		return "no Source takes this title's tags: " + strings.Join(failures, "; "), map[string]any{"sources": skipped}, true, false
	}
	return "no Source answered: " + strings.Join(failures, "; "), map[string]any{"sources": skipped}, true, failed
}

// tagsWhy starts the reason a tagged Source was not asked.
const tagsWhy = "its tags ("

// pauseWords says why a Source is paused in plain words.
func pauseWords(why string) string {
	if strings.HasPrefix(why, tagsWhy) {
		return why
	}
	switch why {
	case KindRateLimited:
		return "slowed down"
	case KindDown:
		return "not reachable"
	case KindLoginRefused:
		return "login refused"
	case KindBroken:
		return "setup broken"
	case KindBlocked:
		return "blocked"
	case BlockBotCheck:
		return "blocked by the site's bot check"
	case BlockSiteBan:
		return "the site refuses HolyArrs's address"
	}
	return "paused"
}

// episodeSet builds the show search for one episode Slot.
func (s *Searcher) episodeSet(ctx context.Context, c slotCtx, out bool) (showSet, *epSlot, error) {
	set, err := s.loadShow(ctx, c.ti, c.ver)
	if err != nil {
		return set, nil, err
	}
	set.single = true
	set.target[c.sl.ID] = true
	if !out {
		set.notOut[c.sl.ID] = true
	}
	for i := range set.eps {
		if set.eps[i].sl != nil && set.eps[i].sl.ID == c.sl.ID {
			return set, &set.eps[i], nil
		}
	}
	return set, nil, fmt.Errorf("episode Slot %d is not in its show", c.sl.ID)
}

// feedShow judges a feed check's releases for one show, per Version. Each
// release is matched to the episodes it holds and chosen or held for every
// looking Slot it fills, best first. It returns how many were chosen, held
// and newly stored.
func (s *Searcher) feedShow(ctx context.Context, by domain.Actor, jobID *int64, cs []slotCtx, fs []found, storedID map[string]int64,
	keyOf func(found) string) (int, int, int, error) {
	chosen, held, stored := 0, 0, 0
	byVer := map[int64][]slotCtx{}
	var order []int64
	for _, c := range cs {
		if len(byVer[c.ver.ID]) == 0 {
			order = append(order, c.ver.ID)
		}
		byVer[c.ver.ID] = append(byVer[c.ver.ID], c)
	}
	rules := s.showRules(ctx)
	for _, vid := range order {
		group := byVer[vid]
		set, err := s.loadShow(ctx, group[0].ti, group[0].ver)
		if err != nil {
			return chosen, held, stored, err
		}
		kept := map[int64]bool{}
		for _, e := range set.eps {
			if e.sl != nil && e.kept {
				kept[e.sl.ID] = true
			}
		}
		heldNow := map[int64]int64{}
		cands := append([]found(nil), fs...)
		for _, c := range group {
			if !kept[c.sl.ID] {
				continue
			}
			set.target[c.sl.ID] = true
			if c.sl.State == domain.SlotHeld && c.sl.HeldReleaseID != nil {
				heldNow[c.sl.ID] = *c.sl.HeldReleaseID
				dup := false
				for _, f := range cands {
					dup = dup || f.relID == *c.sl.HeldReleaseID
				}
				if f, ok := s.storedFound(ctx, *c.sl.HeldReleaseID); ok && !dup {
					cands = append(cands, f)
				}
			}
		}
		if len(set.target) == 0 {
			continue
		}
		for k := range cands {
			if id := storedID[keyOf(cands[k])]; id > 0 && cands[k].relID == 0 {
				cands[k].relID = id
			}
		}
		rels, vs, err := s.judgeShow(ctx, set, cands, false, false, rules)
		if err != nil {
			return chosen, held, stored, err
		}
		picks := s.pickShow(ctx, cands, vs, time.Now().UTC())
		first := int64(0)
		for id := range set.target {
			if first == 0 || id < first {
				first = id
			}
		}
		ids, err := s.storeAll(ctx, by, jobID, "feed", first, cands, rels, vs, false)
		if err != nil {
			return chosen, held, stored, err
		}
		for k := range cands {
			if cands[k].relID == 0 {
				storedID[keyOf(cands[k])] = ids[k]
				stored++
			}
		}
		keep := picks[:0]
		for _, pk := range picks {
			same := pk.held
			for _, id := range pk.slots {
				same = same && heldNow[id] == ids[pk.idx]
			}
			if !same {
				keep = append(keep, pk)
			}
		}
		if beforeSettle != nil {
			beforeSettle(first)
		}
		c1, h1, _ := s.settlePicks(ctx, by, jobID, set.ti.ID, set.labels(), cands, vs, ids, keep, "Feed check")
		chosen, held = chosen+c1, held+h1
	}
	return chosen, held, stored, nil
}
