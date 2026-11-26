package releasename

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// fold maps accented Latin letters to plain ones. Letters it does not list
// (Romanian ș, Vietnamese ữ, Greek ά) are split into letter and mark by
// plainLetter, and the mark is dropped.
var fold = func() map[rune]string {
	m := map[rune]string{}
	add := func(to string, from string) {
		for _, c := range from {
			m[c] = to
		}
	}
	add("a", "àáâãäåāăą")
	add("c", "çćĉċč")
	add("d", "ďđ")
	add("e", "èéêëēĕėęě")
	add("g", "ĝğġģ")
	add("h", "ĥħ")
	add("i", "ìíîïĩīĭįı")
	add("j", "ĵ")
	add("k", "ķ")
	add("l", "ĺļľŀł")
	add("n", "ñńņňŉ")
	add("o", "òóôõöøōŏő")
	add("r", "ŕŗř")
	add("s", "śŝşš")
	add("t", "ţťŧ")
	add("u", "ùúûüũūŭůűų")
	add("w", "ŵ")
	add("y", "ýÿŷ")
	add("z", "źżž")
	m['ß'] = "ss"
	m['þ'] = "th"
	m['ð'] = "d"
	m['æ'] = "ae"
	m['œ'] = "oe"
	m['ς'] = "σ"
	return m
}()

// plainLetter writes a Latin or Greek letter with marks (ș, ơ, ữ, ά) as its
// base letter, so every accented letter is read plain, not only the ones fold
// lists. Other scripts stay as they are (Cyrillic й is its own letter).
func plainLetter(c rune) (string, bool) {
	if c < 0x80 || !(unicode.Is(unicode.Latin, c) || unicode.Is(unicode.Greek, c)) {
		return "", false
	}
	d := norm.NFD.String(string(c))
	if d == string(c) {
		return "", false
	}
	var b strings.Builder
	for _, r := range d {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String(), true
}

// FoldAccents writes accented Latin letters plain and keeps their case
// (Pokémon -> Pokemon, Ávila -> Avila); other letters stay as they are.
func FoldAccents(s string) string {
	var b strings.Builder
	for _, c := range s {
		lower := unicode.ToLower(c)
		f, ok := fold[lower]
		if !ok {
			f, ok = plainLetter(lower)
		}
		switch {
		case !ok:
			b.WriteRune(c)
		case lower != c && f != "":
			r := []rune(f)
			b.WriteString(string(unicode.ToUpper(r[0])) + string(r[1:]))
		default:
			b.WriteString(f)
		}
	}
	return b.String()
}

// CleanName is the form names are matched in: lower case, accents removed,
// punctuation removed, "&" read as "and", and a leading "the", "a" or "an" dropped.
func CleanName(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if f, ok := fold[c]; ok {
			b.WriteString(f)
			continue
		}
		if f, ok := plainLetter(c); ok {
			b.WriteString(f)
			continue
		}
		switch {
		case c == '&':
			b.WriteString(" and ")
		case unicode.IsLetter(c) || unicode.IsDigit(c):
			b.WriteRune(c)
		case unicode.Is(unicode.Mn, c):
			// combining mark: drop
		default:
			b.WriteByte(' ')
		}
	}
	words := strings.Fields(b.String())
	// An "a" before another single letter starts letters written apart
	// ("A.L.F."), not an article.
	acronym := len(words) > 1 && words[0] == "a" && len([]rune(words[1])) == 1 && unicode.IsLetter([]rune(words[1])[0])
	if len(words) > 1 && !acronym && (words[0] == "the" || words[0] == "a" || words[0] == "an") {
		words = words[1:]
	}
	return strings.Join(words, " ")
}
