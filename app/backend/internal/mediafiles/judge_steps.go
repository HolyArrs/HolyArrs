package mediafiles

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

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

// sumPoints adds in 64 bits and stops at MaxPoints either way, so many
// large scores cannot wrap around where int is 32 bits (armv7).
func sumPoints(p releasename.Result, rules PickRulesBody) int {
	points, _ := scoreTraits(p, rules)
	return points
}

// traitHit is one points Trait's test on a Release, kept so the judge
// tests each Trait once and reuses the answer for the total and the reasons.
type traitHit struct {
	ok  bool
	why string
}

// scoreTraits tests every points Trait once; it returns the total (see
// sumPoints) and each Trait's answer, in the order of rules.Traits.
func scoreTraits(p releasename.Result, rules PickRulesBody) (int, []traitHit) {
	var n int64
	hits := make([]traitHit, len(rules.Traits))
	for i, tp := range rules.Traits {
		ok, why := traitMatches(tp.Trait, p)
		hits[i] = traitHit{ok, why}
		if ok {
			n += int64(tp.Points)
		}
	}
	n -= int64(DevicePenalty * len(deviceMisses(p, rules.Devices)))
	return int(max(-MaxPoints, min(MaxPoints, n))), hits
}

// gainOver reports whether points beat cur by at least step, worked out in
// 64 bits so the difference of two large totals cannot wrap around.
func gainOver(points, cur, step int) bool { return int64(points)-int64(cur) >= int64(step) }

func (v *Verdict) safety(rel ReleaseFacts, rules PickRulesBody, step string, set SlotSet) {
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

func (v *Verdict) size(rel ReleaseFacts, rules PickRulesBody, step string, runtime int) {
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
	v.sizeOff = rel.Size - sizeTarget(l, runtime, lo, hi)
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

// langNames names every language the release-name reader knows.
var langNames = map[string]string{"en": "English", "fr": "French", "de": "German", "it": "Italian",
	"es": "Spanish", "ru": "Russian", "ja": "Japanese", "ko": "Korean",
	"pl": "Polish", "nl": "Dutch", "pt": "Portuguese", "sv": "Swedish", "da": "Danish", "no": "Norwegian",
	"fi": "Finnish", "cs": "Czech", "hu": "Hungarian", "el": "Greek", "he": "Hebrew", "tr": "Turkish",
	"ar": "Arabic", "hi": "Hindi", "zh": "Chinese", "th": "Thai", "vi": "Vietnamese", "uk": "Ukrainian",
	"bg": "Bulgarian", "ro": "Romanian", "ca": "Catalan", "lv": "Latvian", "lt": "Lithuanian", "sk": "Slovak", "is": "Icelandic", "fa": "Persian", "bn": "Bengali", "ta": "Tamil", "te": "Telugu", "ml": "Malayalam", "kn": "Kannada", "ka": "Georgian", "ur": "Urdu", "et": "Estonian", "sr": "Serbian", "hr": "Croatian", "sl": "Slovenian", "id": "Indonesian", "ms": "Malay", "sq": "Albanian", "af": "Afrikaans", "mr": "Marathi", "tl": "Filipino", "mk": "Macedonian", "bs": "Bosnian", "pa": "Punjabi", "gu": "Gujarati"}

func (v *Verdict) language(p releasename.Result, rules PickRulesBody, t *TitleRef) {
	langs := p.Languages
	if len(langs) == 0 && rules.Synced.OldApp() && t.OriginalLanguage != "" {
		// As the old apps: no language word means the original language.
		langs = []string{t.OriginalLanguage}
	}
	if len(langs) == 0 && (p.Has("dual") || p.Has("multi")) {
		// DUAL / MULTi with no language word: more than one audio language,
		// so not the original language alone; import reads the audio.
		v.add("hard", "language", "warn", "dual or multi audio with no language named; audio languages get checked at import", 0)
		return
	}
	if len(langs) == 0 {
		// Read as the original language: turned away when only other languages are wanted.
		if o := t.OriginalLanguage; o != "" && !rules.Language.OriginalOK && len(rules.Language.Wanted) > 0 && !slices.Contains(rules.Language.Wanted, o) {
			v.add("hard", "language", "fail", "no language in the name; assumed the original language, "+LanguageName(o)+", not wanted", 0)
			return
		}
		v.add("hard", "language", "warn", "no language in the name; assumed the original language", 0)
		v.originalOnly = rules.Language.OriginalOK && len(rules.Language.Wanted) > 0 && !slices.Contains(rules.Language.Wanted, t.OriginalLanguage)
		return
	}
	keepsOriginal := p.Has("dual") || p.Has("multi") || p.Has("original")
	if p.Has("original") && rules.Synced.OldApp() && t.OriginalLanguage != "" && !slices.Contains(langs, t.OriginalLanguage) {
		// As the old apps: a name that keeps the original language (German
		// DL) is in that language too.
		langs = append(slices.Clip(langs), t.OriginalLanguage)
	}
	for _, l := range langs {
		if slices.Contains(rules.Language.Wanted, l) {
			v.add("hard", "language", "pass", LanguageName(l)+" is wanted", 0)
			return
		}
	}
	for _, l := range langs {
		if rules.Language.OriginalOK && l == t.OriginalLanguage {
			v.add("hard", "language", "pass", LanguageName(l)+" is the original language", 0)
			v.originalOnly = len(rules.Language.Wanted) > 0
			return
		}
	}
	if rules.Language.OriginalOK && keepsOriginal {
		v.add("hard", "language", "pass", "also has the original language", 0)
		v.originalOnly = len(rules.Language.Wanted) > 0
		return
	}
	v.add("hard", "language", "fail", "language "+LanguageName(langs[0])+" not wanted", 0)
}

// LanguageName is the English name of a language code, or the code when it is not known.
func LanguageName(code string) string {
	if n, ok := langNames[code]; ok {
		return n
	}
	return code
}

// LanguageCode is the code for a language name or code a Source sends
// ("French", "fr", "pt-BR"), or "" when it is not one the reader knows.
func LanguageCode(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return ""
	}
	if _, ok := langNames[v]; ok {
		return v
	}
	if base, _, ok := strings.Cut(v, "-"); ok {
		if _, known := langNames[base]; known {
			return base
		}
	}
	if base, _, ok := strings.Cut(v, "("); ok {
		v = strings.TrimSpace(base)
	}
	for code, name := range langNames {
		if strings.ToLower(name) == v {
			return code
		}
	}
	return ""
}

// better applies pick-rules.md section 4. A fix of the same step counts
// first under take_any, with no time limit, so a proper replaces a copy that is already good enough.
func (v *Verdict) better(rel ReleaseFacts, rules PickRulesBody, cur *Current, idx, points, runtime int) {
	curIdx := stepIndex(rules, cur.Step)
	if rankStep(rules, idx) >= rankStep(rules, curIdx) && cur.parse != nil && FileUnderMin(rules, cur.Step, cur.parse.Size, runtime) {
		v.add("better", "under_min", "pass", "the current file is below the smallest size for "+StepWord(cur.Step), 0)
		return
	}
	p := rel.Parse
	pol := rules.ProperPolicy
	fixCounts := p.Fix > cur.Fix && idx == curIdx
	switch pol.Mode {
	case "same_group_only":
		fixCounts = fixCounts && p.Group.Value != "" && strings.EqualFold(p.Group.Value, cur.Group) && points >= cur.Points
	case "prefer_only", "ignore":
		fixCounts = false
	}
	// Steps of one old-app group rank equal from here on.
	low := StepWord(rules.Steps[idx].Quality)
	idx, curIdx = rankStep(rules, idx), rankStep(rules, curIdx)
	if fixCounts && pol.MaxAgeDays > 0 && !cur.ImportedAt.IsZero() && time.Since(cur.ImportedAt) > time.Duration(pol.MaxAgeDays)*24*time.Hour {
		fixCounts = false
	}
	if idx >= curIdx && languageUpgrade(p, rules, cur) {
		v.add("better", "language", "pass", "in a wanted language; the current file is not", 0)
		return
	}
	switch frenchBetter(p, rules, cur) {
	case 1:
		if idx >= curIdx {
			v.add("better", "french_version", "pass", "a wanted French version; the current file is not", 0)
			return
		}
	case -1:
		v.add("better", "french_version", "fail", "the current file has a wanted French version; this one does not", 0)
		return
	}
	// Points first is a choice old apps do not have, so once it is set
	// the rules are judged the way rules made here are.
	if rules.Synced.OldApp() && rules.Order != "points_then_quality" {
		v.betterImported(rules, cur, idx, curIdx, points, fixCounts, low)
		return
	}
	ge := curIdx >= stepIndex(rules, rules.GoodEnoughAt) && (rules.GoodEnoughPoints == nil || cur.Points >= *rules.GoodEnoughPoints)
	switch {
	case idx < curIdx:
		v.add("better", "lower_step", "fail", fmt.Sprintf("%s is lower than the current file's %s", low, StepWord(cur.Step)), 0)
	case fixCounts:
		v.add("better", "fix", "pass", fmt.Sprintf("a fixed copy (fix %d over %d) of the same step", p.Fix, cur.Fix), 0)
	case ge:
		v.add("better", "good_enough_already", "fail", "the current file is already good enough", 0)
	case rules.Order == "points_then_quality":
		if gainOver(points, cur.Points, rules.UpgradeMinPoints) || (points == cur.Points && idx > curIdx) {
			v.add("better", "points", "pass", fmt.Sprintf("%d points over the current %d", points, cur.Points), 0)
		} else {
			v.add("better", "not_better", "fail", "not better than the current file", 0)
		}
	default:
		if idx > curIdx || gainOver(points, cur.Points, rules.UpgradeMinPoints) {
			v.add("better", "higher", "pass", "better than the current file", 0)
		} else {
			v.add("better", "not_better", "fail", "not better than the current file", 0)
		}
	}
}

// FileUnderMin reports whether a File of this step and size is below the
// step's smallest size while the rules replace such Files. An unknown size
// or runtime is never below.
func FileUnderMin(rules PickRulesBody, step string, size int64, runtime int) bool {
	if !rules.ReplaceUnderMin || step == "" || size <= 0 || runtime <= 0 {
		return false
	}
	l, ok := sizeLimit(rules, step)
	if !ok {
		return false
	}
	lo, _ := scaledSizeLimit(l, runtime)
	return size < lo
}

// betterImported is the check for Pick rules brought from an old app, in
// its order: a higher step counts only while the file is below the Good
// enough step, and more points only while the file is below the Good
// enough points, each on its own.
func (v *Verdict) betterImported(rules PickRulesBody, cur *Current, idx, curIdx, points int, fixCounts bool, low string) {
	switch {
	case idx < curIdx:
		v.add("better", "lower_step", "fail", fmt.Sprintf("%s is lower than the current file's %s", low, StepWord(cur.Step)), 0)
	case fixCounts:
		v.add("better", "fix", "pass", "a fixed copy of the same step", 0)
	case idx > curIdx && curIdx < stepIndex(rules, rules.GoodEnoughAt):
		v.add("better", "higher", "pass", "better than the current file", 0)
	case idx > curIdx:
		v.add("better", "good_enough_already", "fail", "the current file's step is already good enough", 0)
	case rules.GoodEnoughPoints != nil && cur.Points >= *rules.GoodEnoughPoints:
		v.add("better", "good_enough_already", "fail", "the current file's points are already good enough", 0)
	case points > cur.Points && gainOver(points, cur.Points, rules.UpgradeMinPoints):
		v.add("better", "points", "pass", fmt.Sprintf("%d points over the current %d", points, cur.Points), 0)
	default:
		v.add("better", "not_better", "fail", "not better than the current file", 0)
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
