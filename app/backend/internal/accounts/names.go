package accounts

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
	rs := []rune(name)
	if rs[0] == ' ' || rs[len(rs)-1] == ' ' || strings.Contains(name, "  ") {
		return "names can't start or end with a space or have two spaces in a row"
	}
	for i, c := range rs {
		// Variation selectors only follow a symbol (an emoji such as a heart).
		if c >= 0xfe00 && c <= 0xfe0f && i > 0 && unicode.IsSymbol(rs[i-1]) {
			continue
		}
		if c == ' ' || (!ignorable(c) && !unicode.In(c, unicode.Cc, unicode.Cf, unicode.Co, unicode.Zs, unicode.Zl, unicode.Zp)) {
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

// ignorable says whether c is a default-ignorable code point outside the
// format class (which is already refused): it draws nothing, so it could hide
// in a name.
func ignorable(c rune) bool {
	switch {
	case c >= 0xfe00 && c <= 0xfe0f, c >= 0xe0100 && c <= 0xe01ef, c >= 0x180b && c <= 0x180f, c >= 0x17b4 && c <= 0x17b5:
		return true
	}
	switch c {
	case 0x034f, 0x115f, 0x1160, 0x3164, 0xffa0, 0x2800:
		return true
	}
	return false
}
