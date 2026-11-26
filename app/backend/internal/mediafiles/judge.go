package mediafiles

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// ReleaseFacts is what the judge needs to know about one search result.
type ReleaseFacts struct {
	Name           string
	Parse          releasename.Result
	Size           int64
	Protocol       string // torrent, usenet or a plugin's own download type
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
	// Tags are the title's, with those of its copies' Library folders that ask only tagged Sources; a feed matches a tagged Source's releases only to titles sharing one.
	Tags []string
	// TagsOnly says an untagged Source serves no copy of the title (already checked against the Sources there are).
	TagsOnly bool
	// RuleTags are the tags of the Pick rules its looking Slots use; a tagged Source sharing one also serves the title.
	RuleTags []string
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
	// parse is the file's name as read, kept so Judge can score it again
	// once it knows the title (its original language and year).
	parse *releasename.Result
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
	Listing        bool // the search only lists Releases; a person picks one, nothing is chosen on its own
	SkipGate       bool // a bulk search with bulk_search_respects_gate off
	// GateDetail replaces the gate's words (an episode or a season that has
	// not aired yet).
	GateDetail string
	// MatchReasons are match lines the caller decided (which episodes a
	// show Release covers, season pack rules); a fail refuses the Release.
	MatchReasons []Reason
	// EpisodeTitles are the names of the episodes a show Release holds;
	// languages read only from their words (or the title's) are dropped.
	EpisodeTitles []string
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
	// originalOnly: the language step passed only on the original language
	// while other languages are wanted (the language wait applies).
	originalOnly bool
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
	// "Local title / Original title" (RuTracker): either side may be the name.
	var sides []string
	if strings.Contains(p.Title.Value, " / ") {
		for _, part := range strings.Split(p.Title.Value, " / ") {
			if c := releasename.CleanName(part); c != "" {
				sides = append(sides, c)
			}
		}
	}
	// The title with its kana spelled out is a second name to try.
	if p.KanaTitle != "" {
		sides = append(sides, releasename.CleanName(p.KanaTitle))
	}
	if w := releasename.AmpReading(name); w != "" {
		sides = append(sides, w)
	}
	var cands []*TitleRef
	for i := range lib {
		t := &lib[i]
		if (t.Kind == "show") != p.IsShow() || t.Kind == "artist" {
			continue // an episode name never matches a movie, nor a movie name a show; artists only music names
		}
		if yearInName(p, t) {
			return Match{Title: t, Conf: releasename.Sure, Detail: t.Name + " by clean name"}
		}
		if nameMatch(t, name) || slices.ContainsFunc(sides, func(n string) bool { return nameMatch(t, n) }) {
			cands = append(cands, t)
		}
	}
	if p.IsShow() {
		cands = withTaggedShows(p, name, lib, cands)
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

// nameMatch says whether a clean release name is one of the Title's names,
// or two of its different names one after the other, as Italian trackers
// write "<local> <original>". Each name's forms are made once, so the
// two-name check costs no more than the plain one on a miss.
func nameMatch(t *TitleRef, name string) bool {
	if tag, rest, ok := strings.Cut(name, " "); ok && leadTags[tag] && nameMatch(t, rest) { // "[ITA] Streghe"
		return true
	}
	type head struct {
		i    int
		rest string
	}
	var heads []head
	for i := 0; i <= len(t.Aliases); i++ {
		a := t.Name
		if i > 0 {
			a = t.Aliases[i-1]
		}
		forms := releasename.NameForms(a)
		if slices.Contains(forms, name) || (strings.IndexFunc(a, unicode.IsLower) < 0 && releasename.SameName(a, name)) { // only an all-capitals name has a second way to match
			return true
		}
		for _, f := range forms {
			if f != "" && len(name) > len(f)+1 && name[len(f)] == ' ' && strings.HasPrefix(name, f) {
				rest := name[len(f)+1:]
				for _, aka := range []string{"aka ", "a k a "} { // "<local> AKA <original>"
					rest = strings.TrimPrefix(rest, aka)
				}
				heads = append(heads, head{i, releasename.CleanName(skipYear(rest, t.Year))}) // "the" can start the second name
			}
		}
	}
	for _, h := range heads {
		first := t.Name
		if h.i > 0 {
			first = t.Aliases[h.i-1]
		}
		for j := 0; j <= len(t.Aliases); j++ {
			b := t.Name
			if j > 0 {
				b = t.Aliases[j-1]
			}
			// the same name twice ("Lost Lost") is not two names
			if j != h.i && releasename.CleanName(b) != releasename.CleanName(first) && releasename.SameName(b, h.rest) {
				return true
			}
		}
	}
	return false
}

// skipYear drops the Title's own year (one off either way) standing between
// two names ("Charmed (1998) Streghe").
func skipYear(rest string, year int) string {
	if w, r, ok := strings.Cut(rest, " "); ok && len(w) == 4 && year > 0 {
		if y, err := strconv.Atoi(w); err == nil && y >= year-1 && y <= year+1 {
			return r
		}
	}
	return rest
}

// leadTags are language tags trackers put before the name ("[ITA] Streghe").
var leadTags = map[string]bool{"ita": true, "eng": true, "multi": true, "fre": true, "french": true, "vff": true,
	"truefrench": true, "ger": true, "german": true, "spa": true, "esp": true, "latino": true, "castellano": true,
	"rus": true, "pol": true, "sub": true, "subita": true}

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
		// A workprint is an unfinished cut, ranked with cam copies.
		return src.Value == "cam" || src.Value == "workprint", "is a cam recording or workprint"
	case "telesync":
		// Telecine, screener and R5 copies are pre-release copies too.
		return slices.Contains([]string{"telesync", "telecine", "screener", "regional"}, src.Value), "is a pre-release copy (telesync, telecine, screener or R5)"
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
	// Device traits: what many TVs, sticks and browsers can't play
	// directly, so the media server would have to convert it.
	case "h264":
		return p.Codec.Value == "h264" && p.Codec.Conf == releasename.Sure, "video is H.264"
	case "dts_audio":
		return strings.HasPrefix(p.Audio.Format, "dts"), "audio is DTS"
	case "truehd_audio":
		return p.Audio.Format == "truehd", "audio is TrueHD"
	case "sdr_4k":
		// 2160p with no HDR word in the name: often worse than a good 1080p.
		return p.Resolution.Value >= 2160 && len(p.HDR.Value) == 0, "is 4K without HDR (SDR)"
	case "dv_no_fallback":
		// Dolby Vision with no HDR10 layer named: players without Dolby
		// Vision show wrong colours.
		hdr := p.HDR.Value
		return slices.Contains(hdr, "dv") && !slices.Contains(hdr, "hdr10") && !slices.Contains(hdr, "hdr10plus"),
			"has Dolby Vision without an HDR10 fallback"
	}
	return false, "unknown trait " + trait
}

// StepFor maps a parse to its ladder step name, or "" when there is none.
func StepFor(p releasename.Result) string {
	if p.Music != nil {
		return musicStep(p.Music)
	}
	res := p.Resolution.Value
	// Sizes between the bands fall to the band below: 1440p is 1080p,
	// 4320p is 2160p, 540p and 576p are SD.
	band := ""
	switch {
	case res >= 2160:
		band = "2160p"
	case res >= 1080:
		band = "1080p"
	case res >= 720:
		band = "720p"
	}
	switch p.Source.Value {
	case "cam", "workprint":
		return "cam"
	case "telesync", "telecine", "screener", "regional":
		return "telesync"
	case "dvd":
		return "sd_dvd"
	case "tv":
		if band == "720p" || band == "1080p" {
			return band + "_tv"
		}
		if band == "2160p" {
			// The ladder has no 2160p TV step; a 2160p TV copy (Sonarr's
			// "HDTV-2160p") is the lowest 2160p step, not SD (#874).
			return "2160p_web"
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
	// No source in the name but a size: guess the TV step for that size,
	// as Sonarr and Radarr do, so groups that leave WEB-DL or BluRay out
	// are not turned down. The ladder has no 2160p TV step, so 2160p goes
	// to the lowest 2160p step. No size and no source stays unknown.
	if p.Source.Value == "" && res > 0 {
		switch band {
		case "":
			return "sd_tv"
		case "2160p":
			return "2160p_web"
		}
		return band + "_tv"
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

// sizeTarget is the size a tie goes toward: the preferred size for this
// runtime when set, else the middle of the band.
func sizeTarget(l SizeLimit, runtime int, lo, hi int64) int64 {
	if l.Preferred <= 0 {
		return sizeMidpoint(lo, hi)
	}
	if !l.PerMinute {
		return l.Preferred
	}
	return saturatingPositiveMul(l.Preferred, runtime)
}

// Judge runs the pipeline of pick-rules.md section 3.1 for one Release.
// Hard steps all run so every refusal is reported at once.
func Judge(rel ReleaseFacts, set SlotSet, rules PickRulesBody) Verdict {
	if set.Match.Title != nil && rel.Parse.OriginalLanguage == "" {
		rel.Parse.OriginalLanguage = set.Match.Title.OriginalLanguage
	}
	if set.Match.Title != nil && len(rel.Parse.Languages) > 0 {
		titles := append([]string{set.Match.Title.Name}, set.EpisodeTitles...)
		rel.Parse.Languages = releasename.WithoutTitleWords(rel.Name, releasename.Hints{Show: set.Match.Title.Kind == "show"}, rel.Parse.Languages, titles...)
	}
	if set.Match.Title != nil && rel.Parse.TitleYear == 0 {
		rel.Parse.TitleYear = set.Match.Title.Year
	}
	p := rel.Parse
	if rel.Size > 0 {
		p.Size = rel.Size
	}
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
	// Scored before the refusals, so Choose a release shows a score for refused ones too.
	points, hits := scoreTraits(p, rules)
	v.Points = points

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
	if GroupBlocked(rules.BlockedGroups, p.Group.Value) {
		v.add("hard", "blocked_group", "fail", fmt.Sprintf("the group %s is blocked in these Pick rules", p.Group.Value), 0)
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
		v.frenchVersion(p, rules)
	}
	if rel.Protocol == "torrent" && set.MinSeeders != nil && rel.Seeders != nil {
		if *rel.Seeders < *set.MinSeeders {
			v.add("hard", "seeders", "fail", seedersLine(*rel.Seeders, *set.MinSeeders, true), 0)
		} else {
			v.add("hard", "seeders", "pass", seedersLine(*rel.Seeders, *set.MinSeeders, false), 0)
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
		v.add("step", "not_allowed", "fail", fmt.Sprintf("%s is not allowed in these Pick rules", StepWord(step)), 0)
	default:
		v.Step = &StepRef{Index: idx, Name: step}
		v.add("step", "allowed", "pass", StepWord(step)+" is allowed", 0)
	}

	// 4. Better than what we have.
	if set.Current != nil && v.Step != nil {
		set.Current = currentWithTitle(set.Current, p, rules)
		v.better(rel, rules, set.Current, idx, points, set.RuntimeMinutes)
	}
	if v.failed() {
		return v.refuse()
	}

	// 5. Traits and points.
	for i, tp := range rules.Traits {
		if hits[i].ok {
			eff := "add"
			if tp.Points < 0 {
				eff = "subtract"
			}
			v.add("points", "trait:"+tp.Trait, eff, hits[i].why, tp.Points)
		}
	}
	for _, m := range deviceMisses(p, rules.Devices) {
		v.add("points", "device:"+m.Feature, "subtract", m.why(), -DevicePenalty)
	}
	v.Points = points
	if points < rules.MinPoints {
		v.add("points", "below_min", "fail", fmt.Sprintf("%d points, needs at least %d", points, rules.MinPoints), 0)
		return v.refuse()
	}
	if rules.MinPoints == 0 {
		v.add("points", "total", "pass", fmt.Sprintf("%d points", points), points)
	} else {
		v.add("points", "total", "pass", fmt.Sprintf("%d points (needs at least %d)", points, rules.MinPoints), points)
	}

	// 6. Good enough.
	ge := rankStep(rules, idx) >= stepIndex(rules, rules.GoodEnoughAt)
	if rules.GoodEnoughPoints != nil && points < *rules.GoodEnoughPoints {
		ge = false
	}
	v.GoodEnough = ge
	switch {
	case ge:
		v.add("good_enough", "reached", "pass", "reaches good enough, which starts at "+StepWord(rules.GoodEnoughAt), 0)
	case rankStep(rules, idx) >= stepIndex(rules, rules.GoodEnoughAt):
		v.add("good_enough", "not_reached", "warn", fmt.Sprintf("below good enough, which needs %d points", *rules.GoodEnoughPoints), 0)
	default:
		v.add("good_enough", "not_reached", "warn", "below good enough, which starts at "+StepWord(rules.GoodEnoughAt), 0)
	}

	// 7. Protocol wait.
	wait := rules.Protocol.WaitMinutesTorrent
	if rel.Protocol != "torrent" { // a plugin's own download type waits like usenet
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
	if v.languageWait(rel, rules, set) {
		return v
	}
	v.Passed, v.Outcome = true, "chosen"
	return v
}

// seedersLine is a Release's seeders reason, "1 seeder, at least 1" or "5 seeders, needs at least 10";
// the page shows it in the picked language (server-lines seeders keys).
func seedersLine(n, min int, failed bool) string {
	word := "seeders"
	if n == 1 {
		word = "seeder"
	}
	if failed {
		return fmt.Sprintf("%d %s, needs at least %d", n, word, min)
	}
	return fmt.Sprintf("%d %s, at least %d", n, word, min)
}
