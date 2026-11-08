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

func sumPoints(p releasename.Result, rules PickRulesBody) int {
	n := 0
	for _, tp := range rules.Traits {
		if ok, _ := traitMatches(tp.Trait, p); ok {
			n += tp.Points
		}
	}
	return n
}

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

func (v *Verdict) language(p releasename.Result, rules PickRulesBody, t *TitleRef) {
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
// first under take_any, with no time limit, so a proper replaces a copy that is already good enough.
func (v *Verdict) better(rel ReleaseFacts, rules PickRulesBody, cur *Current, idx, points int) {
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
