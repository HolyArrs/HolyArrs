package mediafiles

import (
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var frenchVersionChoices = []string{releasename.FrenchFrance, releasename.FrenchQuebec, releasename.FrenchInternational, releasename.FrenchBoth}

// frenchFit says how a French version meets the Pick rules: 0 a
// wanted version, 1 not known from the name, 2 a known version not wanted,
// -1 the rules do not ask for a version (French not wanted or no list).
func frenchFit(version string, rules PickRulesBody) int {
	want := rules.Language.FrenchVersions
	if len(want) == 0 || !slices.Contains(rules.Language.Wanted, "fr") {
		return -1
	}
	switch {
	case version == "":
		return 1
	case slices.Contains(want, version):
		return 0
	case version == releasename.FrenchBoth && (slices.Contains(want, releasename.FrenchFrance) || slices.Contains(want, releasename.FrenchQuebec)):
		return 0 // VF2 holds both dubs
	}
	return 2
}

// frenchVersion checks a Release's French version against the rules: a
// known version that is not wanted is refused; no version is allowed.
func (v *Verdict) frenchVersion(p releasename.Result, rules PickRulesBody) {
	fv := releasename.FrenchVersion(p.Original)
	switch frenchFit(fv, rules) {
	case 0:
		v.add("hard", "french_version", "pass", fv+" is a wanted French version", 0)
	case 1:
		if slices.Contains(p.Languages, "fr") || p.Has("multi") {
			v.add("hard", "french_version", "warn", "no French version in the name", 0)
		}
	case 2:
		v.add("hard", "french_version", "fail", "French version "+fv+" not wanted", 0)
	}
}

// frenchBetter compares the French versions of a Release and the current
// file: 1 the Release has a wanted one and the file does not, -1 the file
// has one and the Release does not, 0 no difference.
func frenchBetter(rel releasename.Result, rules PickRulesBody, cur *Current) int {
	if cur == nil || cur.parse == nil {
		return 0
	}
	r := frenchFit(releasename.FrenchVersion(rel.Original), rules)
	c := frenchFit(releasename.FrenchVersion(cur.parse.Original), rules)
	switch {
	case r < 0 || r == c:
		return 0
	case r == 0:
		return 1
	case c == 0:
		return -1
	}
	return 0
}
