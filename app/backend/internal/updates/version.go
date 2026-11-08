package updates

import (
	"strconv"
	"strings"
)

// Newer says whether version a is newer than b (v1.2.3 style; anything
// else is never newer).
func Newer(a, b string) bool {
	pa, oka := parts(a)
	pb, okb := parts(b)
	if !oka || !okb {
		return false
	}
	for i := range 3 {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	// Same numbers: a pre-release (v1.2.0-rc1) is older than the plain
	// version, and two pre-releases are not ordered.
	return !IsPreRelease(a) && IsPreRelease(b)
}

// IsPreRelease says a version carries a pre-release suffix (v1.2.0-rc1).
func IsPreRelease(v string) bool { return strings.Contains(strings.TrimSpace(v), "-") }

func parts(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-")
	ps := strings.Split(v, ".")
	if len(ps) == 0 || len(ps) > 3 {
		return out, false
	}
	for i, p := range ps {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
