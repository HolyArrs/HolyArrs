package judge

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Release is what the judge needs to know about one search result.
type Release struct {
	Name           string
	Parse          releasename.Result
	Size           int64
	Protocol       string // torrent, usenet
	Seeders        *int
	PublishedAt    time.Time
	InfoHash       string
	Files          []string
	SourcePriority int // lower wins
	// Dangerous: a Download of this Release (by info hash or name) failed
	// before because its files were a program (#41).
	Dangerous bool
}

// TitleRef is a library Title as the matcher sees it.
type TitleRef struct {
	ID               int64
	Name             string
	Aliases          []string
	Year             int
	OriginalLanguage string
	Runtime          int    // minutes
	Kind             string // "show" for a show, "artist" for an artist; "" or "movie" for a movie
	// Tags are the title's (#108); a feed matches a tagged Source's releases only to titles sharing one.
	Tags []string
}

// Match is the result of matching a parse to the library.
type Match struct {
	Title  *TitleRef
	Conf   releasename.Conf
	Detail string
}

// Current is the File a Slot already has.
type Current struct {
	Step       string
	Points     int
	Group      string
	Fix        int
	ImportedAt time.Time
}

// SlotSet is the Slots a Release matched, with what judging needs about them.
type SlotSet struct {
	Match          Match
	SlotIDs        []int64
	RuntimeMinutes int
	Out            bool // the Release gate is open (movies) or the air date passed
	MinSeeders     *int
	TurnedDown     []string // names or info hashes turned down for these Slots
	Current        *Current
	HandSearch     bool // a person started a single-Item search
	SkipGate       bool // a bulk search with bulk_search_respects_gate off
	// GateDetail replaces the gate's words (an episode or a season that has
	// not aired yet).
	GateDetail string
	// MatchReasons are match lines the caller decided (which episodes a
	// show Release covers, season pack rules); a fail refuses the Release.
	MatchReasons []Reason
	// OutSince is when the Items came out: a movie's first release day, the
	// last air time of the episodes held, an album's release day. nil: not known.
	OutSince *time.Time
	// FloorEpisodes is how many episodes a Release holds when none of
	// their lengths is known (RuntimeMinutes 0): the fake-size floor then
	// counts Checks.EpisodeMinutes for each.
	FloorEpisodes int
	// Checks are the early and fake-size checks (4.3.5); the zero value
	// checks nothing.
	Checks Checks
}

// Checks turn on the early and fake-size checks (settings
// refuse_early_releases, early_release_grace_hours, refuse_tiny_releases).
type Checks struct {
	Early bool          // a Release posted before OutSince (less Grace) is refused
	Grace time.Duration // time zones and early air times
	Tiny  bool          // a Release far too small for what it says it holds is refused
	// HoldUnknown holds a Release whose size or dates are not known for a
	// person's choice in automatic searches; off, it only warns.
	HoldUnknown bool
	// EpisodeMinutes is the length taken for an episode of unknown length
	// by the fake-size floor (default 20).
	EpisodeMinutes int
}

// Reason is one line of a Verdict.
type Reason struct {
	Step   string `json:"step"`   // match, hard, step, better, points, good_enough, wait, rank, parse
	Rule   string `json:"rule"`   // stable code within the step
	Effect string `json:"effect"` // pass, fail, add, subtract, warn, hold, rank
	Points int    `json:"points,omitempty"`
	Detail string `json:"detail"`
}

// Code is the reason's stable code, "step.rule".
func (r Reason) Code() string { return r.Step + "." + r.Rule }

// StepRef names a ladder step.
type StepRef struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

// Verdict is the judged outcome of one Release for one Slot set.
type Verdict struct {
	SlotIDs    []int64    `json:"slot_ids"`
	Outcome    string     `json:"outcome"` // chosen, held, passed_over, refused
	Passed     bool       `json:"passed"`
	Step       *StepRef   `json:"step"`
	Points     int        `json:"points"`
	GoodEnough bool       `json:"good_enough"`
	HoldUntil  *time.Time `json:"hold_until"`
	Rank       int        `json:"rank"`
	Reasons    []Reason   `json:"reasons"`

	sizeOff int64 // distance from the middle of the size band, for ranking
}

func (v *Verdict) add(step, rule, effect, detail string, pts int) {
	v.Reasons = append(v.Reasons, Reason{Step: step, Rule: rule, Effect: effect, Points: pts, Detail: detail})
}

func (v *Verdict) failed() bool {
	for _, r := range v.Reasons {
		if r.Effect == "fail" {
			return true
		}
	}
	return false
}

// MatchTitle finds the library Title a parse names (pick-rules.md 2.5, steps 2 and 3).
func MatchTitle(p releasename.Result, lib []TitleRef) Match {
	if p.IsMusic() {
		return matchArtist(p, lib)
	}
	name := releasename.CleanName(p.Title.Value)
	var cands []*TitleRef
	for i := range lib {
		t := &lib[i]
		if (t.Kind == "show") != p.IsShow() || t.Kind == "artist" {
			continue // an episode name never matches a movie, nor a movie name a show; artists only music names
		}
		if releasename.CleanName(t.Name) == name {
			cands = append(cands, t)
			continue
		}
		for _, a := range t.Aliases {
			if releasename.CleanName(a) == name {
				cands = append(cands, t)
				break
			}
		}
	}
	if len(cands) == 0 {
		return Match{Conf: releasename.Unknown, Detail: fmt.Sprintf("no Title in the library is called %q", p.Title.Value)}
	}
	if p.IsShow() {
		return matchShow(p, cands)
	}
	if p.Year.Value == 0 {
		if len(cands) == 1 {
			return Match{Title: cands[0], Conf: releasename.Guess,
				Detail: fmt.Sprintf("%s (%d) by name only; the name gives no year", cands[0].Name, cands[0].Year)}
		}
		return Match{Conf: releasename.Unknown, Detail: fmt.Sprintf("%d Titles are called %q and the name gives no year", len(cands), p.Title.Value)}
	}
	for _, t := range cands {
		if t.Year == p.Year.Value {
			return Match{Title: t, Conf: releasename.Sure, Detail: fmt.Sprintf("%s (%d) by clean name and year", t.Name, t.Year)}
		}
	}
	for _, t := range cands {
		if d := t.Year - p.Year.Value; d == 1 || d == -1 {
			return Match{Title: t, Conf: releasename.Likely,
				Detail: fmt.Sprintf("%s (%d) by clean name; the name says %d", t.Name, t.Year, p.Year.Value)}
		}
	}
	return Match{Conf: releasename.Unknown, Detail: fmt.Sprintf("no Title called %q matches year %d", p.Title.Value, p.Year.Value)}
}

// safetyTraits are checked by the safety step, so the never step leaves them out.
var safetyTraits = map[string]bool{"sample": true, "executable": true, "password_protected": true, "disc_image": true}

// traitMatches says whether a named Trait matches. Points Traits need a sure
// field (a "likely" web_dl from a bare "WEB" earns nothing).
func traitMatches(trait string, p releasename.Result) (bool, string) {
	if id, ok := domain.OwnTraitID(trait); ok {
		return ownMatches(id, p)
	}
	if p.Music != nil {
		return musicTrait(trait, p)
	}
	src := p.Source
	switch trait {
	case "hevc":
		return p.Codec.Value == "h265" && p.Codec.Conf == releasename.Sure, "video is HEVC"
	case "ddp_audio":
		return p.Audio.Format == "eac3", "audio is DDP (E-AC-3)"
	case "web_dl":
		return src.Value == "web_dl" && src.Conf == releasename.Sure, "source is WEB-DL"
	case "upscaled":
		return p.Has("upscaled"), "marked as upscaled"
	case "dubbed_only":
		return p.Has("dubbed"), "dubbed only"
	case "hardcoded_subs":
		return p.Has("hardcoded_subs"), "has hardcoded subtitles"
	case "3d":
		return p.Has("three_d"), "is 3D"
	case "cam":
		return src.Value == "cam", "is a cam recording"
	case "telesync":
		return src.Value == "telesync", "is a telesync recording"
	case "remux":
		return src.Value == "remux", "is a remux"
	case "av1":
		return p.Codec.Value == "av1", "video is AV1"
	case "dv":
		return slices.Contains(p.HDR.Value, "dv"), "has Dolby Vision"
	case "hdr10":
		return slices.Contains(p.HDR.Value, "hdr10"), "has HDR10"
	case "hdr10plus":
		return slices.Contains(p.HDR.Value, "hdr10plus"), "has HDR10+"
	case "atmos":
		return p.Audio.Atmos, "has Atmos"
	}
	return false, "unknown trait " + trait
}

// StepFor maps a parse to its ladder step name, or "" when there is none.
func StepFor(p releasename.Result) string {
	if p.Music != nil {
		return musicStep(p.Music)
	}
	res := p.Resolution.Value
	band := map[int]string{720: "720p", 1080: "1080p", 2160: "2160p"}[res]
	switch p.Source.Value {
	case "cam":
		return "cam"
	case "telesync":
		return "telesync"
	case "dvd":
		return "sd_dvd"
	case "tv":
		if band == "720p" || band == "1080p" {
			return band + "_tv"
		}
		return "sd_tv"
	case "web_dl", "web_rip":
		if band == "" {
			return "sd_tv"
		}
		return band + "_web"
	case "bluray", "disc_image":
		if band == "" {
			return "sd_dvd"
		}
		return band + "_bluray"
	case "remux":
		if band == "720p" || band == "" {
			return "1080p_remux"
		}
		return band + "_remux"
	}
	return ""
}

func stepIndex(rules domain.PickRulesBody, name string) int {
	for i, s := range rules.Steps {
		if s.Quality == name {
			return i
		}
	}
	return -1
}

func sizeLimit(rules domain.PickRulesBody, step string) (domain.SizeLimit, bool) {
	if l, ok := rules.SizeLimits[step]; ok {
		return l, true
	}
	l, ok := rules.SizeLimits["*"]
	return l, ok
}

// Believable size per hour by resolution (pick-rules.md 2.3 gives 1.5 GB/h
// for 2160p; 1080p and 720p are our own bands).
var believablePerHour = map[int]float64{2160: 1.5, 1080: 0.5, 720: 0.25}

func gbOf(n int64) float64 { return float64(n) / float64(1<<30) }

func saturatingPositiveMul(n int64, factor int) int64 {
	if n <= 0 || factor <= 0 {
		return 0
	}
	f := int64(factor)
	if n > math.MaxInt64/f {
		return math.MaxInt64
	}
	return n * f
}

func scaledSizeLimit(l domain.SizeLimit, runtime int) (int64, int64) {
	lo, hi := l.Min, l.Max
	if !l.PerMinute {
		return lo, hi
	}
	lo = saturatingPositiveMul(lo, runtime)
	if hi > 0 { // zero means no upper limit
		hi = saturatingPositiveMul(hi, runtime)
	}
	return lo, hi
}

func sizeMidpoint(lo, hi int64) int64 { return lo + (hi-lo)/2 }

// Judge runs the pipeline of pick-rules.md section 3.1 for one Release.
// Hard steps all run so every refusal is reported at once.
func Judge(rel Release, set SlotSet, rules domain.PickRulesBody) Verdict {
	p := rel.Parse
	v := Verdict{SlotIDs: set.SlotIDs}

	// Parse warnings.
	if p.Resolution.Conf == releasename.Guess {
		v.add("parse", "resolution_guess", "warn", strings.Join(p.Notes, "; "), 0)
	}

	// 1. Match.
	switch {
	case set.Match.Title == nil:
		v.add("match", "none", "fail", set.Match.Detail, 0)
		return v.refuse()
	case set.Match.Conf == releasename.Guess:
		v.add("match", "needs_hand_pick", "fail", set.Match.Detail+"; pick the Title by hand", 0)
		return v.refuse()
	case set.Match.Conf == releasename.Likely:
		v.add("match", "year_near", "warn", set.Match.Detail, 0)
	default:
		v.add("match", "title", "pass", set.Match.Detail, 0)
	}
	v.Reasons = append(v.Reasons, set.MatchReasons...)
	if v.failed() {
		return v.refuse()
	}
	if len(set.SlotIDs) == 0 {
		v.add("match", "no_wanted_slot", "fail", "no wanted Slot for this Title", 0)
		return v.refuse()
	}
	title := set.Match.Title
	step := StepFor(p)

	// 2. Hard refusals, in the spec order: never, must have, safety, size,
	// language, seeders, gate, turned down.
	for _, tr := range rules.Never {
		if safetyTraits[tr] {
			continue
		}
		if ok, why := traitMatches(tr, p); ok {
			v.add("hard", "never:"+tr, "fail", why, 0)
		}
	}
	if !hasFail(v, "hard", "never:") {
		v.add("hard", "never", "pass", "nothing from the never list", 0)
	}
	for _, tr := range rules.MustHave {
		if ok, why := traitMatches(tr, p); !ok {
			v.add("hard", "must_have:"+tr, "fail", "must have: "+why, 0)
		}
	}
	v.safety(rel, rules, step, set)
	v.size(rel, rules, step, set.RuntimeMinutes)
	if p.Music != nil {
		v.add("hard", "language", "pass", "music: the language is not checked", 0)
	} else {
		v.language(p, rules, title)
	}
	if rel.Protocol == "torrent" && set.MinSeeders != nil && rel.Seeders != nil {
		if *rel.Seeders < *set.MinSeeders {
			v.add("hard", "seeders", "fail", fmt.Sprintf("%d seeders, needs at least %d", *rel.Seeders, *set.MinSeeders), 0)
		} else {
			v.add("hard", "seeders", "pass", fmt.Sprintf("%d seeders, at least %d", *rel.Seeders, *set.MinSeeders), 0)
		}
	}
	switch {
	case set.Out && set.GateDetail != "":
		v.add("hard", "gate", "pass", set.GateDetail, 0)
	case set.Out:
		v.add("hard", "gate", "pass", "the Title is out", 0)
	case set.GateDetail != "" && !set.HandSearch && !set.SkipGate:
		v.add("hard", "gate", "fail", set.GateDetail, 0)
	case set.HandSearch:
		// A single-Item search by a person may skip the gate, and it says so.
		v.add("hard", "gate_skipped", "warn", "the Title is not out yet; the Release gate is skipped because a person searched for it", 0)
	case set.SkipGate:
		v.add("hard", "gate_skipped", "warn", "the Title is not out yet; the Release gate is skipped because bulk searches are set to skip it", 0)
	default:
		v.add("hard", "gate", "fail", "the Title is not out yet", 0)
	}
	if slices.Contains(set.TurnedDown, rel.Name) || (rel.InfoHash != "" && slices.Contains(set.TurnedDown, rel.InfoHash)) {
		v.add("hard", "turned_down", "fail", "this Release was turned down for this Slot before", 0)
	} else {
		v.add("hard", "turned_down", "pass", "not turned down before", 0)
	}

	// 3. Step on the ladder.
	idx := stepIndex(rules, step)
	switch {
	case step == "" && p.Music != nil:
		v.add("step", "none", "fail", "no ladder step: the name gives no audio format or bit rate", 0)
	case idx < 0 && p.Music != nil:
		v.add("step", "none", "fail", "these Pick rules have no music steps; give the artist music Pick rules", 0)
	case step == "" || idx < 0:
		v.add("step", "none", "fail", "no ladder step: the source is unknown", 0)
	case !rules.Steps[idx].Allowed:
		v.Step = &StepRef{Index: idx, Name: step}
		v.add("step", "not_allowed", "fail", fmt.Sprintf("%s is not allowed in these Pick rules", step), 0)
	default:
		v.Step = &StepRef{Index: idx, Name: step}
		v.add("step", "allowed", "pass", step+" is allowed", 0)
	}

	// 4. Better than what we have.
	points := sumPoints(p, rules)
	if set.Current != nil && v.Step != nil {
		v.better(rel, rules, set.Current, idx, points)
	}
	if v.failed() {
		return v.refuse()
	}

	// 5. Traits and points.
	for _, tp := range rules.Traits {
		if ok, why := traitMatches(tp.Trait, p); ok {
			eff := "add"
			if tp.Points < 0 {
				eff = "subtract"
			}
			v.add("points", "trait:"+tp.Trait, eff, why, tp.Points)
		}
	}
	v.Points = points
	if points < rules.MinPoints {
		v.add("points", "below_min", "fail", fmt.Sprintf("%d points, needs at least %d", points, rules.MinPoints), 0)
		return v.refuse()
	}
	v.add("points", "total", "pass", fmt.Sprintf("%d points, at least %d", points, rules.MinPoints), points)

	// 6. Good enough.
	ge := idx >= stepIndex(rules, rules.GoodEnoughAt)
	if rules.GoodEnoughPoints != nil && points < *rules.GoodEnoughPoints {
		ge = false
	}
	v.GoodEnough = ge
	if ge {
		v.add("good_enough", "reached", "pass", "reaches good enough ("+rules.GoodEnoughAt+")", 0)
	} else {
		v.add("good_enough", "not_reached", "warn", "below good enough ("+rules.GoodEnoughAt+")", 0)
	}

	// 7. Protocol wait.
	wait := rules.Protocol.WaitMinutesTorrent
	if rel.Protocol == "usenet" {
		wait = rules.Protocol.WaitMinutesUsenet
	}
	until := rel.PublishedAt.Add(time.Duration(wait) * time.Minute)
	switch {
	case wait == 0:
		v.add("wait", "none", "pass", "no wait for "+rel.Protocol, 0)
	case set.HandSearch:
		v.add("wait", "skipped", "pass", "wait skipped: a person searched for this", 0)
	case ge && rules.Protocol.SkipWaitAtGoodEnough:
		v.add("wait", "skipped", "pass", "wait skipped: good enough", 0)
	case time.Now().Before(until):
		v.HoldUntil = &until
		v.Passed, v.Outcome = true, "held"
		v.add("wait", "hold", "hold", fmt.Sprintf("held until %s (%d min wait for %s)", until.UTC().Format(time.RFC3339), wait, rel.Protocol), 0)
		return v
	default:
		v.add("wait", "over", "pass", "wait is over", 0)
	}
	v.Passed, v.Outcome = true, "chosen"
	return v
}

func (v Verdict) refuse() Verdict {
	v.Passed, v.Outcome = false, "refused"
	return v
}

func hasFail(v Verdict, step, prefix string) bool {
	for _, r := range v.Reasons {
		if r.Step == step && r.Effect == "fail" && strings.HasPrefix(r.Rule, prefix) {
			return true
		}
	}
	return false
}

func sumPoints(p releasename.Result, rules domain.PickRulesBody) int {
	n := 0
	for _, tp := range rules.Traits {
		if ok, _ := traitMatches(tp.Trait, p); ok {
			n += tp.Points
		}
	}
	return n
}

func (v *Verdict) safety(rel Release, rules domain.PickRulesBody, step string, set SlotSet) {
	p := rel.Parse
	runtime := set.RuntimeMinutes
	start := len(v.Reasons)
	never := func(t string) bool { return slices.Contains(rules.Never, t) }
	if p.Has("double_extension") {
		v.add("hard", "safety:double_extension", "fail", "file name hides a program", 0)
	}
	if rel.Dangerous {
		v.add("hard", "safety:dangerous_files", "fail", "its files were a program when it was downloaded before", 0)
	}
	if p.Has("executable") {
		what := "a program file"
		for _, f := range rel.Files {
			if j := strings.LastIndexByte(f, '.'); j >= 0 && slices.Contains([]string{"exe", "scr", "bat", "cmd", "msi", "lnk", "js", "vbs", "ps1"}, strings.ToLower(f[j+1:])) {
				what = "a program file (" + f + ")"
				break
			}
		}
		v.add("hard", "safety:executable", "fail", "contains "+what+", not a video", 0)
	}
	if p.Has("password_hint") {
		v.add("hard", "safety:password_protected", "fail", "archive needs a password", 0)
	}
	if p.Has("sample") && (rel.Size == 0 || rel.Size < 150<<20) {
		v.add("hard", "safety:sample", "fail", "is a sample", 0)
	}
	if p.Has("extras") {
		v.add("hard", "safety:extras_only", "fail", "only extras", 0)
	}
	if p.Source.Value == "disc_image" && never("disc_image") {
		v.add("hard", "safety:disc_image", "fail", "full disc image", 0)
	}
	if l, ok := sizeLimit(rules, step); ok && runtime > 0 && rel.Size > 0 {
		lo, hi := scaledSizeLimit(l, runtime)
		if rel.Size < lo/5 || (hi > 0 && rel.Size > saturatingPositiveMul(hi, 5)) {
			what := fmt.Sprintf("%dp", p.Resolution.Value)
			if p.Music != nil {
				what = step
			}
			v.add("hard", "safety:fake_by_size", "fail",
				fmt.Sprintf("%s in %.0f MB is not believable", what, float64(rel.Size)/(1<<20)), 0)
		}
	}
	v.earlyAndTiny(rel, set)
	if len(v.Reasons) == start {
		v.add("hard", "safety", "pass", "no safety problems", 0)
	}
}

func (v *Verdict) size(rel Release, rules domain.PickRulesBody, step string, runtime int) {
	l, ok := sizeLimit(rules, step)
	if !ok {
		v.add("hard", "size", "pass", "no size limits", 0)
		return
	}
	if l.PerMinute && runtime <= 0 {
		v.add("hard", "size", "warn", "runtime unknown, size not checked", 0)
		return
	}
	lo, hi := scaledSizeLimit(l, runtime)
	if rel.Parse.Music != nil {
		v.musicSize(rel.Size, lo, hi)
		return
	}
	got := gbOf(rel.Size)
	v.sizeOff = rel.Size - sizeMidpoint(lo, hi)
	if v.sizeOff < 0 {
		v.sizeOff = -v.sizeOff
	}
	switch {
	case rel.Size < lo:
		v.add("hard", "size", "fail", fmt.Sprintf("%.2f GB is under the %.2f GB minimum", got, gbOf(lo)), 0)
	case hi > 0 && rel.Size > hi:
		v.add("hard", "size", "fail", fmt.Sprintf("%.2f GB is over the %.2f GB maximum", got, gbOf(hi)), 0)
	default:
		v.add("hard", "size", "pass", fmt.Sprintf("%.2f GB within %.2f to %.2f GB", got, gbOf(lo), gbOf(hi)), 0)
	}
	if runtime > 0 {
		if min, ok := believablePerHour[rel.Parse.Resolution.Value]; ok {
			if per := got / (float64(runtime) / 60); per < min {
				v.add("hard", "size_mismatch", "warn",
					fmt.Sprintf("%.2f GB per hour is small for %dp", per, rel.Parse.Resolution.Value), 0)
			}
		}
	}
}

var langNames = map[string]string{"en": "English", "fr": "French", "de": "German", "it": "Italian",
	"es": "Spanish", "ru": "Russian", "ja": "Japanese", "ko": "Korean"}

func (v *Verdict) language(p releasename.Result, rules domain.PickRulesBody, t *TitleRef) {
	langs := p.Languages
	if len(langs) == 0 {
		v.add("hard", "language", "warn", "no language in the name; assumed the original language", 0)
		return
	}
	keepsOriginal := p.Has("dual") || p.Has("multi")
	for _, l := range langs {
		if slices.Contains(rules.Language.Wanted, l) {
			v.add("hard", "language", "pass", langName(l)+" is wanted", 0)
			return
		}
		if rules.Language.OriginalOK && l == t.OriginalLanguage {
			v.add("hard", "language", "pass", langName(l)+" is the original language", 0)
			return
		}
	}
	if rules.Language.OriginalOK && keepsOriginal {
		v.add("hard", "language", "pass", "also has the original language", 0)
		return
	}
	v.add("hard", "language", "fail", "language "+langName(langs[0])+" not wanted", 0)
}

func langName(code string) string {
	if n, ok := langNames[code]; ok {
		return n
	}
	return code
}

// better applies pick-rules.md section 4. A fix of the same step counts
// first under take_any (decision of 2026-10-12, mirroring the common
// default), so a proper replaces a copy that is already good enough.
func (v *Verdict) better(rel Release, rules domain.PickRulesBody, cur *Current, idx, points int) {
	curIdx := stepIndex(rules, cur.Step)
	p := rel.Parse
	pol := rules.ProperPolicy
	fixCounts := p.Fix > cur.Fix && idx == curIdx
	switch pol.Mode {
	case "same_group_only":
		fixCounts = fixCounts && p.Group.Value != "" && strings.EqualFold(p.Group.Value, cur.Group) && points >= cur.Points
	case "prefer_only", "ignore":
		fixCounts = false
	}
	if fixCounts && pol.MaxAgeDays > 0 && !cur.ImportedAt.IsZero() && time.Since(cur.ImportedAt) > time.Duration(pol.MaxAgeDays)*24*time.Hour {
		fixCounts = false
	}
	ge := curIdx >= stepIndex(rules, rules.GoodEnoughAt) && (rules.GoodEnoughPoints == nil || cur.Points >= *rules.GoodEnoughPoints)
	switch {
	case idx < curIdx:
		v.add("better", "lower_step", "fail", fmt.Sprintf("%s is lower than the current file (%s)", rules.Steps[idx].Quality, cur.Step), 0)
	case fixCounts:
		v.add("better", "fix", "pass", fmt.Sprintf("a fixed copy (fix %d over %d) of the same step", p.Fix, cur.Fix), 0)
	case ge:
		v.add("better", "good_enough_already", "fail", "the current file is already good enough", 0)
	case rules.Order == "points_then_quality":
		if points-cur.Points >= rules.UpgradeMinPoints || (points == cur.Points && idx > curIdx) {
			v.add("better", "points", "pass", fmt.Sprintf("%d points over the current %d", points, cur.Points), 0)
		} else {
			v.add("better", "not_better", "fail", "not better than the current file", 0)
		}
	default:
		if idx > curIdx || points-cur.Points >= rules.UpgradeMinPoints {
			v.add("better", "higher", "pass", "better than the current file", 0)
		} else {
			v.add("better", "not_better", "fail", "not better than the current file", 0)
		}
	}
}

// seederBand puts seeders into the bands of section 3.2.
func seederBand(s *int) int {
	if s == nil {
		return 0
	}
	switch {
	case *s >= 200:
		return 4
	case *s >= 50:
		return 3
	case *s >= 10:
		return 2
	case *s >= 1:
		return 1
	}
	return 0
}

// Rank orders the passed Verdicts (section 3.2). The first becomes chosen
// (unless it is held) and each loser gets a reason naming the key it lost on.
// rels and vs are parallel.
func Rank(rels []Release, vs []Verdict, rules domain.PickRulesBody) { rank(rels, vs, rules, false) }

// RankPacksFirst ranks like Rank, but a passing season pack comes before
// any Release that is not one (setting season_pack_prefer = always).
func RankPacksFirst(rels []Release, vs []Verdict, rules domain.PickRulesBody) {
	rank(rels, vs, rules, true)
}

func rank(rels []Release, vs []Verdict, rules domain.PickRulesBody, packsFirst bool) {
	var idx []int
	for i := range vs {
		if vs[i].Passed {
			idx = append(idx, i)
		}
	}
	type key struct {
		name   string
		a, b   func(i int) int
		higher bool
	}
	step := func(i int) int { return vs[i].Step.Index }
	pts := func(i int) int { return vs[i].Points }
	keys := []key{{"quality", step, step, true}, {"points", pts, pts, true}}
	if rules.Order == "points_then_quality" {
		keys[0], keys[1] = keys[1], keys[0]
	}
	if packsFirst {
		pack := func(i int) int {
			if isPack(rels[i].Parse) {
				return 1
			}
			return 0
		}
		keys = append([]key{{"season pack", pack, pack, true}}, keys...)
	}
	middle := func(i int) int { return -int(vs[i].sizeOff >> 20) }
	cmp := func(a, b int) (int, string) {
		for _, k := range keys {
			if x, y := k.a(a), k.a(b); x != y {
				if x > y {
					return -1, fmt.Sprintf("on %s (%d vs %d)", k.name, y, x)
				}
				return 1, fmt.Sprintf("on %s (%d vs %d)", k.name, x, y)
			}
		}
		pa, pb := rels[a].Parse, rels[b].Parse
		if pa.Group.Value != "" && strings.EqualFold(pa.Group.Value, pb.Group.Value) && pa.Fix != pb.Fix {
			if pa.Fix > pb.Fix {
				return -1, "on fix number"
			}
			return 1, "on fix number"
		}
		if pref := rules.Protocol.Prefer; pref != "either" && rels[a].Protocol != rels[b].Protocol {
			if rels[a].Protocol == pref {
				return -1, "on preferred protocol"
			}
			return 1, "on preferred protocol"
		}
		if c, why := episodeOrder(pa, pb); c != 0 {
			return c, why
		}
		if x, y := len(vs[a].SlotIDs), len(vs[b].SlotIDs); x != y && !pa.IsShow() {
			if x > y {
				return -1, "on Slots covered"
			}
			return 1, "on Slots covered"
		}
		if x, y := rels[a].SourcePriority, rels[b].SourcePriority; x != y {
			if x < y {
				return -1, "on Source priority"
			}
			return 1, "on Source priority"
		}
		if x, y := seederBand(rels[a].Seeders), seederBand(rels[b].Seeders); x != y {
			if x > y {
				return -1, "on seeders"
			}
			return 1, "on seeders"
		}
		if x, y := middle(a), middle(b); x != y {
			if x > y {
				return -1, "on size"
			}
			return 1, "on size"
		}
		if ta, tb := rels[a].PublishedAt, rels[b].PublishedAt; !ta.Equal(tb) {
			if ta.Before(tb) {
				return -1, "on published time"
			}
			return 1, "on published time"
		}
		return 0, ""
	}
	sort.SliceStable(idx, func(x, y int) bool { c, _ := cmp(idx[x], idx[y]); return c < 0 })
	for pos, i := range idx {
		vs[i].Rank = pos + 1
		if pos == 0 {
			vs[i].add("rank", "first", "rank", fmt.Sprintf("ranked 1 of %d", len(idx)), 0)
			continue
		}
		w := idx[0]
		_, why := cmp(w, i)
		if why == "" {
			why = "on order found"
		}
		vs[i].Outcome = "passed_over"
		vs[i].add("rank", "lost", "rank", fmt.Sprintf("lost to %s %s", rels[w].Name, why), 0)
	}
}

// ReachesGoodEnough reports whether a file with this release name meets the
// rules' Good enough step (and points, when set). It is used when the Pick
// rules of a Version change.
func ReachesGoodEnough(releaseName string, rules domain.PickRulesBody) bool {
	p := releasename.Parse(releaseName, hintsFor(rules))
	idx := stepIndex(rules, StepFor(p))
	if idx < 0 || idx < stepIndex(rules, rules.GoodEnoughAt) {
		return false
	}
	return rules.GoodEnoughPoints == nil || sumPoints(p, rules) >= *rules.GoodEnoughPoints
}

// CurrentOf describes a File already in the Library from its release name,
// for the "better than what we have" step (pick-rules.md section 4).
func CurrentOf(releaseName string, importedAt time.Time, rules domain.PickRulesBody) Current {
	p := releasename.Parse(releaseName, hintsFor(rules))
	return Current{Step: StepFor(p), Points: sumPoints(p, rules), Group: p.Group.Value, Fix: p.Fix, ImportedAt: importedAt}
}

// StepOf is the ladder step a name parses to and its place in the rules'
// ladder (-1 when the step is unknown or not on the ladder).
func StepOf(name string, rules domain.PickRulesBody) (string, int) {
	step := StepFor(releasename.Parse(name, hintsFor(rules)))
	if step == "" {
		return "", -1
	}
	return step, stepIndex(rules, step)
}
