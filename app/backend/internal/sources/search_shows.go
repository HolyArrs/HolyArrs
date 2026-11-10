package sources

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

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
	ref mediafiles.EpisodeRef
	it  mediafiles.Item
	// sl is nil when the episode has no Slot in this Version.
	sl  *mediafiles.Slot
	cur *mediafiles.Current
	// kept means the episode, its season, the Version and the show are all kept up to date.
	kept bool
	// soon means aired, or airing within a day, when season packs may be taken.
	soon    bool
	runtime int
}

// showSet is a show search: the show's episodes in one Version and the Slots asked for.
type showSet struct {
	ti     mediafiles.Title
	ver    mediafiles.Version
	pr     mediafiles.PickRules
	eps    []epSlot
	target map[int64]bool
	// notOut holds target Slots that were not out yet; a hand search skips that gate.
	notOut map[int64]bool
	// single is an episode search, where season packs are refused.
	single bool
}

// EpisodeLabel names an episode item: S01E02, or its air day for a daily show.
func EpisodeLabel(it mediafiles.Item) string {
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
func currentOf(ctx context.Context, db *sql.DB, sl mediafiles.Slot, pr mediafiles.PickRules) *mediafiles.Current {
	if sl.FileID == nil {
		return nil
	}
	var f mediafiles.File
	if err := mediafiles.LoadFile(ctx, db, *sl.FileID, &f); err != nil || f.State != "present" {
		return nil
	}
	name := strings.TrimSuffix(path.Base(f.RelativePath), path.Ext(f.RelativePath))
	if f.ReleaseName != nil && *f.ReleaseName != "" {
		name = *f.ReleaseName
	}
	if cur := mediafiles.CurrentOf(mediafiles.JudgeName(name, f.MediaInfo), f.Size, f.AddedAt, pr.Body); cur.Step != "" {
		return &cur
	}
	if mediafiles.IsMusicRules(pr.Body) {
		if word := trackFormat(ctx, db, f); word != "" {
			if cur := mediafiles.CurrentOf(name+" ["+word+"]", f.Size, f.AddedAt, pr.Body); cur.Step != "" {
				return &cur
			}
		}
	}
	return nil
}

// trackFormat is the format word of a track file: its extension, or for a file
// that may hold either kind the codec read from its library folder. A lossless
// copy must stay protected however it was named.
func trackFormat(ctx context.Context, db *sql.DB, f mediafiles.File) string {
	ext := strings.ToLower(path.Ext(f.RelativePath))
	switch ext {
	case "":
		return ""
	case ".m4a", ".mka", ".mp4":
		lib, err := mediafiles.LibraryPathOfVersion(ctx, db, f.VersionID)
		if err != nil {
			return ""
		}
		info, err := mediafiles.Inspect(filepath.Join(lib, filepath.FromSlash(f.RelativePath)))
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
func (s *Searcher) loadShow(ctx context.Context, ti mediafiles.Title, ver mediafiles.Version) (showSet, error) {
	db := s.Store.Read()
	set := showSet{ti: ti, ver: ver, target: map[int64]bool{}, notOut: map[int64]bool{}}
	if err := mediafiles.LoadPickRules(ctx, db, ver.PickRulesID, &set.pr); err != nil {
		return set, err
	}
	parts, err := mediafiles.PartsOfTitle(ctx, db, ti.ID)
	if err != nil {
		return set, err
	}
	partKept := map[int64]bool{}
	for _, p := range parts {
		partKept[p.ID] = p.KeepUpToDate && p.DroppedAt == nil
	}
	its, err := mediafiles.EpisodesOfTitle(ctx, db, ti.ID)
	if err != nil {
		return set, err
	}
	slots, err := mediafiles.VersionSlotsOfTitle(ctx, db, ver.ID, ti.ID)
	if err != nil {
		return set, err
	}
	byItem := map[int64]*mediafiles.Slot{}
	for i := range slots {
		byItem[slots[i].ItemID] = &slots[i]
	}
	soon := time.Now().Add(24 * time.Hour)
	for _, it := range its {
		e := epSlot{it: it, sl: byItem[it.ID]}
		e.ref = mediafiles.EpisodeRef{ItemID: it.ID}
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

func (set showSet) ref() mediafiles.TitleRef {
	r := mediafiles.TitleRef{ID: set.ti.ID, Name: set.ti.Name, Aliases: set.ti.Aliases, Kind: "show"}
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

var lookingNow = []mediafiles.SlotState{mediafiles.SlotWanted, mediafiles.SlotBetterWanted}

// judgeShow judges every release for a show search and ranks them.
func (s *Searcher) judgeShow(ctx context.Context, set showSet, all []found, hand, skipGate bool, rules showRules) ([]mediafiles.ReleaseFacts, []mediafiles.Verdict, error) {
	bad := s.dangerousSeen(ctx)
	refs := make([]mediafiles.EpisodeRef, len(set.eps))
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
	rels := make([]mediafiles.ReleaseFacts, len(all))
	vs := make([]mediafiles.Verdict, len(all))
	for i, f := range all {
		p := releasename.Parse(f.item.Title, releasename.Hints{Size: f.item.Size, Show: true})
		rels[i] = mediafiles.ReleaseFacts{Name: f.item.Title, Parse: p, Size: f.item.Size, Protocol: f.item.Protocol, Seeders: f.item.Seeders,
			PublishedAt: f.item.Published, InfoHash: f.item.InfoHash, SourcePriority: f.src.Priority, Dangerous: bad.has(listingHash(f.item.InfoHash, f.item.Link), f.item.Title)}
		ss := mediafiles.SlotSet{Match: mediafiles.MatchTitle(p, []mediafiles.TitleRef{tref}), MinSeeders: f.src.MinSeeders, HandSearch: hand, SkipGate: skipGate, Out: true,
			Checks: checks}
		if ss.Match.Title != nil {
			set.lines(&ss, rels[i], refs, byItem, rules, turnedFor)
		}
		vs[i] = mediafiles.Judge(rels[i], ss, set.pr.Body)
	}
	if rules.packsFirst {
		mediafiles.RankPacksFirst(rels, vs, set.pr.Body)
	} else {
		mediafiles.Rank(rels, vs, set.pr.Body)
	}
	return rels, vs, nil
}

func matchFail(rule, detail string) mediafiles.Reason {
	return mediafiles.Reason{Step: "match", Rule: rule, Effect: "fail", Detail: detail}
}

// lines works out which episodes and Slots a show release covers and applies
// the season pack rules.
func (set showSet) lines(ss *mediafiles.SlotSet, rel mediafiles.ReleaseFacts, refs []mediafiles.EpisodeRef, byItem map[int64]*epSlot, rules showRules,
	turnedFor func(int64) []string) {
	p := rel.Parse
	found, why := mediafiles.MatchEpisodes(p, set.order(), refs)
	if why != "" {
		ss.MatchReasons = append(ss.MatchReasons, matchFail("episode", why))
		return
	}
	covered := make([]*epSlot, 0, len(found))
	for _, r := range found {
		covered = append(covered, byItem[r.ItemID])
	}
	ss.MatchReasons = append(ss.MatchReasons, mediafiles.Reason{Step: "match", Rule: "episode", Effect: "pass", Detail: "holds " + coverLabel(covered)})
	pack := mediafiles.IsPack(p)
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
			if e.kept && e.sl.State != mediafiles.SlotNotOutYet {
				noGain = append(noGain, e.ref.Label()+" is "+strings.ReplaceAll(string(e.sl.State), "_", " "))
			}
			continue
		}
		if l := turnedFor(e.sl.ID); slices.Contains(l, rel.Name) || rel.InfoHash != "" && slices.Contains(l, rel.InfoHash) {
			turnedSome = append(turnedSome, e.ref.Label())
			targetTurned = targetTurned || isTarget
			continue
		}
		if !mediafiles.Gains(rel, set.pr.Body, e.cur) {
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
		ss.MatchReasons = append(ss.MatchReasons, mediafiles.Reason{Step: "match", Rule: "turned_down_some", Effect: "warn",
			Detail: "turned down before for " + strings.Join(turnedSome, ", ") + "; not used for those"})
	}
	if len(noGain) > 0 {
		r := mediafiles.Reason{Step: "match", Rule: "pack_share", Effect: "warn", Detail: "not every episode would gain: " + strings.Join(noGain, "; ")}
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
func (s *Searcher) pickShow(ctx context.Context, all []found, vs []mediafiles.Verdict, now time.Time) []showPick {
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
			vs[i].Reasons = append(vs[i].Reasons, mediafiles.Reason{Step: "rank", Rule: "source_limit", Effect: "rank", Detail: why})
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
