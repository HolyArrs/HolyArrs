package library

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"strings"
	"unicode/utf8"
)

// maxAliases caps the other names kept on one Title.
const maxAliases = 30

// genericNames are common words that cannot tell one Title from another.
var genericNames = map[string]bool{
	"movie": true, "movies": true, "film": true, "films": true, "show": true, "series": true,
	"untitled": true, "the movie": true, "the film": true, "the series": true,
}

// usableAlias says whether a clean name is safe to match a release against:
// not a number, not a generic word, and not one short word.
func usableAlias(clean string) bool {
	if clean == "" || genericNames[clean] {
		return false
	}
	digits := true
	for _, c := range clean {
		if c < '0' || c > '9' {
			digits = false
			break
		}
	}
	if digits {
		return false
	}
	return strings.Contains(clean, " ") || utf8.RuneCountInString(clean) >= 5
}

// mergeAliases keeps the names a Title already has, then adds the usable new
// ones, skipping repeats (same clean name) and the Title's own name, up to
// maxAliases.
func MergeAliases(have []string, name string, more []string) []string {
	out := make([]string, 0, len(have)+len(more))
	seen := map[string]bool{releasename.CleanName(name): true}
	add := func(list []string, filter bool) {
		for _, a := range list {
			a = strings.TrimSpace(a)
			c := releasename.CleanName(a)
			if c == "" || seen[c] || len(out) >= maxAliases || (filter && !usableAlias(c)) {
				continue
			}
			seen[c] = true
			out = append(out, a)
		}
	}
	add(have, false)
	add(more, true)
	return out
}
