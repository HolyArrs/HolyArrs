package mediafiles

import (
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Own traits are the traits a person makes. The judge reads them from
// one shared set, so every path that judges a Release (searches, the feed,
// Choose a release, Good enough after an edit) sees the same ones; the app
// sets it at start and after each change (SetOwnTraits).

type ownTrait struct {
	name  string
	group bool
	plain []string // lowercased words
	pats  []*regexp.Regexp
}

var ownTraits atomic.Pointer[map[int64]ownTrait]

// SetOwnTraits replaces the own traits the judge knows. A term that is not
// a readable pattern is skipped (CheckOwnTrait refuses those on save).
func SetOwnTraits(ts []OwnTrait) {
	m := make(map[int64]ownTrait, len(ts))
	for _, t := range ts {
		o := ownTrait{name: t.Name, group: t.Match == "group"}
		for _, term := range t.Terms {
			if p, ok := TermPattern(term); ok {
				if re, err := regexp.Compile("(?i)" + p); err == nil {
					o.pats = append(o.pats, re)
				}
				continue
			}
			if term = strings.ToLower(strings.TrimSpace(term)); term != "" {
				o.plain = append(o.plain, term)
			}
		}
		m[t.ID] = o
	}
	ownTraits.Store(&m)
}

// ownMatches is traitMatches for an own trait: the name test finds any term
// in the Release name, the group test needs the release group to be one of
// the words or fit one of the patterns.
func ownMatches(id int64, p releasename.Result) (bool, string) {
	all := ownTraits.Load()
	if all == nil {
		return false, "unknown trait"
	}
	t, ok := (*all)[id]
	if !ok {
		return false, "a trait that was deleted"
	}
	if t.group {
		g := p.Group.Value
		if g == "" {
			return false, t.name + ": the release group is not known"
		}
		for _, w := range t.plain {
			if strings.EqualFold(g, w) {
				return true, t.name + ": release group " + g
			}
		}
		for _, re := range t.pats {
			if re.MatchString(g) {
				return true, t.name + ": release group " + g
			}
		}
		return false, t.name + ": release group " + g + " is not one of them"
	}
	low := strings.ToLower(p.Original)
	for _, w := range t.plain {
		if strings.Contains(low, w) {
			return true, t.name + ": the name has " + w
		}
	}
	for _, re := range t.pats {
		if m := re.FindString(p.Original); m != "" {
			return true, t.name + ": the name has " + m
		}
	}
	return false, t.name + ": none of its words are in the name"
}
