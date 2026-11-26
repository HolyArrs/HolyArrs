package mediafiles

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Rank orders the passed Verdicts (section 3.2). The first becomes chosen
// (unless it is held) and each loser gets a reason naming the key it lost on.
// rels and vs are parallel.
func Rank(rels []ReleaseFacts, vs []Verdict, rules PickRulesBody) { rank(rels, vs, rules, false) }

// RankPacksFirst ranks like Rank, but a passing season pack comes before
// any Release that is not one (setting season_pack_prefer = always).
func RankPacksFirst(rels []ReleaseFacts, vs []Verdict, rules PickRulesBody) {
	rank(rels, vs, rules, true)
}

func rank(rels []ReleaseFacts, vs []Verdict, rules PickRulesBody, packsFirst bool) {
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
	step := func(i int) int { return rankStep(rules, vs[i].Step.Index) }
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
	if rules.Language.WaitDays > 0 {
		// With a language wait, a wanted language wins over the original.
		lang := func(i int) int {
			if vs[i].originalOnly {
				return 0
			}
			return 1
		}
		keys = append([]key{{"language", lang, lang, true}}, keys...)
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
		// Compared across groups (as Sonarr does): a same-group-only key made the
		// order depend on the other releases in the list.
		if rules.ProperPolicy.Mode != "ignore" && pa.Fix != pb.Fix {
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
		// Two packs of equal quality, points and protocol: the one covering
		// more wanted episodes wins (a multi-season pack over a one-season
		// pack when both are allowed), so a search for many missing seasons
		// takes one pack instead of several. SlotIDs hold only the
		// episodes the pack would fill.
		if pa.IsShow() && pb.IsShow() && isPack(pa) && isPack(pb) {
			if x, y := len(vs[a].SlotIDs), len(vs[b].SlotIDs); x != y {
				if x > y {
					return -1, fmt.Sprintf("on wanted episodes covered (%d vs %d)", y, x)
				}
				return 1, fmt.Sprintf("on wanted episodes covered (%d vs %d)", x, y)
			}
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
		// A Release loses only to the best one ahead of it that holds one of
		// its Slots; one for other episodes is no rival (the pickers take
		// one per Slot), so with none it stays the best for its own.
		w := -1
		for _, j := range idx[:pos] {
			if shareSlot(vs[j].SlotIDs, vs[i].SlotIDs) {
				w = j
				break
			}
		}
		if w < 0 {
			vs[i].add("rank", "first", "rank", fmt.Sprintf("ranked 1 for what it holds (%d of %d overall)", pos+1, len(idx)), 0)
			continue
		}
		_, why := cmp(w, i)
		if why == "" {
			why = "on order found"
		}
		vs[i].Outcome = "passed_over"
		vs[i].add("rank", "lost", "rank", fmt.Sprintf("lost to %s %s", rels[w].Name, why), 0)
	}
}

// shareSlot says two Releases hold a Slot in common. One with no Slots
// (not matched to any yet) counts as a rival of every other, as before.
func shareSlot(a, b []int64) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

// ReachesGoodEnough reports whether a file with this release name meets the
// rules' Good enough step (and points, when set). It is used when the Pick
// rules of a Version change.
func ReachesGoodEnough(releaseName string, rules PickRulesBody) bool {
	p := releasename.Parse(releaseName, hintsFor(rules))
	idx := rankStep(rules, stepIndex(rules, StepFor(p)))
	if idx < 0 || idx < stepIndex(rules, rules.GoodEnoughAt) {
		return false
	}
	return rules.GoodEnoughPoints == nil || sumPoints(p, rules) >= *rules.GoodEnoughPoints
}

// CurrentOf describes a File already in the Library from its release name,
// for the "better than what we have" step (pick-rules.md section 4). size
// is the File's own size in bytes (0: not known), so size traits score it
// the way they score a new Release.
func CurrentOf(releaseName string, size int64, importedAt time.Time, rules PickRulesBody) Current {
	return CurrentOfFile(releaseName, size, importedAt, nil, rules)
}

// CurrentOfFile is CurrentOf for a File whose audio languages are known
// (FileLanguages): language trait tests read them before the name.
func CurrentOfFile(releaseName string, size int64, importedAt time.Time, audio []string, rules PickRulesBody) Current {
	p := releasename.Parse(releaseName, hintsFor(rules))
	p.Size = size
	p.AudioLanguages = audio
	return Current{Step: StepFor(p), Points: sumPoints(p, rules), Group: p.Group.Value, Fix: p.Fix, ImportedAt: importedAt, parse: &p}
}

// currentWithTitle scores the current file again with what the Release's
// match says of the title (original language, year) when the file's own
// name lacks them, so language and year trait tests read the file as they
// read a Release. Without those tests the points are the same.
func currentWithTitle(cur *Current, rel releasename.Result, rules PickRulesBody) *Current {
	if cur.parse == nil {
		return cur
	}
	p := *cur.parse
	lang := p.OriginalLanguage == "" && rel.OriginalLanguage != ""
	year := p.Year.Value <= 0 && p.TitleYear == 0 && rel.TitleYear > 0
	if !lang && !year {
		return cur
	}
	if p.OriginalLanguage == "" {
		p.OriginalLanguage = rel.OriginalLanguage
	}
	if p.TitleYear == 0 {
		p.TitleYear = rel.TitleYear
	}
	c := *cur
	c.Points = sumPoints(p, rules)
	return &c
}

// StepOf is the ladder step a name parses to and its place in the rules'
// ladder (-1 when the step is unknown or not on the ladder).
func StepOf(name string, rules PickRulesBody) (string, int) {
	step := StepFor(releasename.Parse(name, hintsFor(rules)))
	if step == "" {
		return "", -1
	}
	return step, stepIndex(rules, step)
}
