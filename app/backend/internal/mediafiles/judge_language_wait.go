package mediafiles

import (
	"fmt"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// languageWait holds a Release that passed only on the original language
// while the rules want other languages and set a language wait: it
// waits WaitDays from when the Title came out (else from when the Release
// was posted). A person's own search skips the wait. It reports whether
// the Release is held.
func (v *Verdict) languageWait(rel ReleaseFacts, rules PickRulesBody, set SlotSet) bool {
	days := rules.Language.WaitDays
	if days <= 0 || !v.originalOnly {
		return false
	}
	if set.HandSearch {
		v.add("wait", "language_skipped", "pass", "language wait skipped: a person searched for this", 0)
		return false
	}
	from := rel.PublishedAt
	if set.OutSince != nil && !set.OutSince.IsZero() {
		from = *set.OutSince
	}
	until := from.Add(time.Duration(days) * 24 * time.Hour)
	if time.Now().Before(until) {
		v.HoldUntil = &until
		v.Passed, v.Outcome = true, "held"
		v.add("wait", "language_hold", "hold", fmt.Sprintf("only the original language; held until %s (%d day wait for a wanted language)", until.UTC().Format(time.RFC3339), days), 0)
		return true
	}
	v.add("wait", "language_over", "pass", "language wait is over; the original language is taken", 0)
	return false
}

// languageUpgrade says whether a Release in a wanted language replaces a
// current file that has none of them, when the rules set a language wait.
func languageUpgrade(rel releasename.Result, rules PickRulesBody, cur *Current) bool {
	want := rules.Language.Wanted
	if rules.Language.WaitDays <= 0 || !rules.Language.OriginalOK || len(want) == 0 || cur == nil || cur.parse == nil {
		return false
	}
	has := func(langs []string) bool {
		return slices.ContainsFunc(langs, func(l string) bool { return slices.Contains(want, l) })
	}
	if !has(rel.Languages) {
		return false
	}
	p := *cur.parse
	if p.OriginalLanguage == "" {
		p.OriginalLanguage = rel.OriginalLanguage
	}
	langs := languagesOf(p)
	return len(langs) > 0 && !has(langs)
}
