package mediafiles

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// ReleaseFacts is what the judge needs to know about one search result.
type ReleaseFacts struct {
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
	// before because its files were a program.
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
	// Tags are the title's; a feed matches a tagged Source's releases only to titles sharing one.
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
	Out            bool // the ReleaseFacts gate is open (movies) or the air date passed
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
	Early bool          // a ReleaseFacts posted before OutSince (less Grace) is refused
	Grace time.Duration // time zones and early air times
	Tiny  bool          // a ReleaseFacts far too small for what it says it holds is refused
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
	if id, ok := OwnTraitID(trait); ok {
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

func stepIndex(rules PickRulesBody, name string) int {
	for i, s := range rules.Steps {
		if s.Quality == name {
			return i
		}
	}
	return -1
}

func sizeLimit(rules PickRulesBody, step string) (SizeLimit, bool) {
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

func scaledSizeLimit(l SizeLimit, runtime int) (int64, int64) {
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
func Judge(rel ReleaseFacts, set SlotSet, rules PickRulesBody) Verdict {
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
