package auth

import (
	"strings"
	"unicode"
)

// nameProblem says what is wrong with a new account name, or "". Names with
// hidden characters (zero-width, direction marks, control characters, odd
// spaces) are refused, so nobody can pick a name that looks exactly like
// someone else's. Only names being made are checked; existing ones keep
// working.
func nameProblem(name string) string {
	if name == "" || len(name) > 64 {
		return "pick a name of 1 to 64 characters"
	}
	if strings.IndexFunc(name, func(c rune) bool {
		return c != ' ' && unicode.In(c, unicode.Cc, unicode.Cf, unicode.Co, unicode.Zs, unicode.Zl, unicode.Zp)
	}) >= 0 {
		return "names can't have hidden characters or special spaces; use letters, numbers and plain spaces"
	}
	return ""
}
