package auth

import "unicode"

// nameProblem says what is wrong with a new account name, or "". Names with
// hidden characters (zero-width, direction marks, control characters, odd
// spaces) are refused, so nobody can pick a name that looks exactly like
// someone else's. Only names being made are checked; existing ones keep
// working.
func nameProblem(name string) string {
	if name == "" || len(name) > 64 {
		return "pick a name of 1 to 64 characters"
	}
	rs := []rune(name)
	for i, c := range rs {
		if c == ' ' || !unicode.In(c, unicode.Cc, unicode.Cf, unicode.Co, unicode.Zs, unicode.Zl, unicode.Zp) {
			continue
		}
		// Zero-width joiner and non-joiner are part of how Persian, Indic and
		// emoji names are spelled; allowed between two non-ASCII letters,
		// marks or symbols, so they cannot hide inside a plain Latin name.
		if (c == 0x200c || c == 0x200d) && i > 0 && i < len(rs)-1 && joins(rs[i-1]) && joins(rs[i+1]) {
			continue
		}
		return "names can't have hidden characters or special spaces; use letters, numbers and plain spaces"
	}
	return ""
}

func joins(c rune) bool {
	return c > 0x7f && (unicode.IsLetter(c) || unicode.IsMark(c) || unicode.IsSymbol(c))
}
