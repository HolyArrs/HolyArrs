package downloads

import (
	"unicode"
	"unicode/utf8"
)

// upperFirst puts the first letter of s in capitals.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}
