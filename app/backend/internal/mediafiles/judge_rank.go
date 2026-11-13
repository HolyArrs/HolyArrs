package mediafiles

import (
	"fmt"
	"sort"
	"strings"
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
func ReachesGoodEnough(releaseName string, rules PickRulesBody) bool {
	p := releasename.Parse(releaseName, hintsFor(rules))
	idx := stepIndex(rules, StepFor(p))
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
