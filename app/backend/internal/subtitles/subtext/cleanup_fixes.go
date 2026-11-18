package subtext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits that keep the text steps linear on hostile input.
const (
	maxTagLen  = 200 // bytes in one <...> or {\...} tag
	maxSpanLen = 300 // bytes in one [...] or (...) sound description
	maxSpeaker = 30  // bytes in one speaker name before its colon
)

// tagLen returns the length of the formatting tag at the start of s, or 0.
// It knows the SRT tags i, b, u, s and font, and override blocks such as
// {\an8} that some SRT files borrow from ASS.
func tagLen(s string) int {
	if len(s) < 3 {
		return 0
	}
	switch {
	case s[0] == '<':
		j := strings.IndexByte(s[:min(len(s), maxTagLen)], '>')
		if j < 0 {
			return 0
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(s[1:j], "/"), " ")
		switch strings.ToLower(name) {
		case "i", "b", "u", "s", "font":
			return j + 1
		}
	case s[0] == '{' && s[1] == '\\':
		if j := strings.IndexByte(s[:min(len(s), maxTagLen)], '}'); j >= 0 {
			return j + 1
		}
	}
	return 0
}

// stripTags returns s without its formatting tags.
func stripTags(s string) string {
	if !strings.ContainsAny(s, "<{") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if n := tagLen(s[i:]); n > 0 {
			i += n - 1
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// leadingTags returns the tags at the start of line.
func leadingTags(line string) string {
	i := 0
	for i < len(line) {
		n := tagLen(line[i:])
		if n == 0 {
			break
		}
		i += n
	}
	return line[:i]
}

// blank reports whether text shows nothing once tags, spaces and lone
// dialogue dashes are gone.
func blank(text string) bool {
	return strings.Trim(stripTags(text), " \t\r\n-") == ""
}

// tidyLines trims the lines of a cue and drops the ones that show nothing. A
// dropped line's tags move to the line before it, or to the next line when it
// was first, so an italic opened there still closes.
func tidyLines(text string) string {
	var lines []string
	carry := ""
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if blank(l) {
			if lines != nil {
				lines[len(lines)-1] += stripText(l)
			} else {
				carry += stripText(l)
			}
			continue
		}
		lines = append(lines, carry+l)
		carry = ""
	}
	return strings.Join(lines, "\n")
}

// stripText keeps only the tags of s.
func stripText(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); {
		if n := tagLen(s[i:]); n > 0 {
			sb.WriteString(s[i : i+n])
			i += n
			continue
		}
		i++
	}
	return sb.String()
}

// changedTidy returns text when fixed is the same, and fixed with its lines
// tidied otherwise, so a step that found nothing never reflows a cue.
func changedTidy(text, fixed string) string {
	if fixed == text {
		return text
	}
	return tidyLines(fixed)
}

// removeTags drops the formatting tags.
func removeTags(text, _ string) string {
	return changedTidy(text, stripTags(text))
}

// isEmoji reports whether r is a pictograph. Music notes stay, since
// subtitles use them to mark song lyrics.
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF, r >= 0xE0020 && r <= 0xE007F:
		return true
	case r >= 0x2669 && r <= 0x266C:
		return false
	case r >= 0x2600 && r <= 0x27BF, r >= 0x23E9 && r <= 0x23FA:
		return true
	}
	return r == 0x231A || r == 0x231B || r == 0x2B50 || r == 0x2B55 || r == 0xFE0F || r == 0x20E3
}

// removeEmoji drops pictographs, with the joiners that glue several into one.
func removeEmoji(text, _ string) string {
	var sb strings.Builder
	prev := false
	for _, r := range text {
		if isEmoji(r) || (prev && r == 0x200D) {
			prev = true
			continue
		}
		prev = false
		sb.WriteRune(r)
	}
	out := sb.String()
	if out == text {
		return text
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = collapseSpaces(l)
	}
	return tidyLines(strings.Join(lines, "\n"))
}

// removeHI drops sound descriptions in square brackets or parentheses and
// speaker names in capitals. When two dashed speakers become one, the dash of
// the one left goes too.
func removeHI(text, _ string) string {
	dashed := 0
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(stripTags(l)), "-") {
			dashed++
		}
	}
	out := dropSpans(text, '[', ']')
	out = dropSpans(out, '(', ')')
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = dropSpeaker(l)
	}
	out = changedTidy(text, strings.Join(lines, "\n"))
	if out != text && dashed >= 2 && !strings.Contains(out, "\n") {
		out = dropLeadDash(out)
	}
	return out
}

// dropSpans removes every open...close span of at most maxSpanLen bytes. The
// next close is remembered, so a run of opens with no close stays linear.
func dropSpans(s string, open, close byte) string {
	if strings.IndexByte(s, open) < 0 {
		return s
	}
	var sb strings.Builder
	next := -1
	for i := 0; i < len(s); i++ {
		if s[i] == open {
			if next < i {
				next = len(s)
				if j := strings.IndexByte(s[i:], close); j >= 0 {
					next = i + j
				}
			}
			if next < len(s) && next-i <= maxSpanLen {
				i = next
				continue
			}
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// dropSpeaker drops a speaker name such as "JOHN:" or "- MAN 2:" from the
// start of line and keeps the dialogue dash. Only names in capitals count,
// which keeps "Note: ..." and times such as 10:30.
func dropSpeaker(line string) string {
	lead := leadingTags(line)
	t := strings.TrimLeft(line[len(lead):], " ")
	dash := ""
	if rest, ok := strings.CutPrefix(t, "-"); ok {
		dash, t = "- ", strings.TrimLeft(rest, " ")
	}
	colon := strings.IndexByte(t[:min(len(t), maxSpeaker+1)], ':')
	if colon < 1 || !speakerName(t[:colon]) {
		return line
	}
	return lead + dash + strings.TrimLeft(t[colon+1:], " ")
}

// speakerName reports whether s reads as a name in capitals: it starts with a
// letter, has at least two letters and none in lower case.
func speakerName(s string) bool {
	letters := 0
	for i, r := range s {
		switch {
		case unicode.IsLetter(r):
			if unicode.IsLower(r) {
				return false
			}
			letters++
		case i == 0:
			return false
		case !unicode.IsDigit(r) && !strings.ContainsRune(" .'-#&", r):
			return false
		}
	}
	return letters >= 2
}

// dropLeadDash drops the dialogue dash at the start of line.
func dropLeadDash(line string) string {
	lead := leadingTags(line)
	t := strings.TrimLeft(line[len(lead):], " ")
	if rest, ok := strings.CutPrefix(t, "-"); ok {
		return lead + strings.TrimLeft(rest, " ")
	}
	return line
}

// wordRune reports whether r belongs to a word for the word-level fixes. The
// bar is in, since OCR reads a capital I as one.
func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '’' || r == '|'
}

// eachWord calls fix on every word of line and puts back what it returns.
// Tags such as <font color="#F0F0F0"> are passed over untouched.
func eachWord(line string, fix func(w string) string) string {
	var sb strings.Builder
	for i := 0; i < len(line); {
		if n := tagLen(line[i:]); n > 0 {
			sb.WriteString(line[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		if !wordRune(r) {
			sb.WriteString(line[i : i+size])
			i += size
			continue
		}
		j := i
		for j < len(line) {
			r, s := utf8.DecodeRuneInString(line[j:])
			if !wordRune(r) || tagLen(line[j:]) > 0 {
				break
			}
			j += s
		}
		sb.WriteString(fix(line[i:j]))
		i = j
	}
	return sb.String()
}

// eachLine runs fix over every line of text.
func eachLine(text string, fix func(string) string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = fix(l)
	}
	return strings.Join(lines, "\n")
}

// ocrFixes mends letters that OCR of picture subtitles often mistakes: a zero
// between letters becomes O, and in English a lone l, or l starting I'm, It's
// and the like, becomes I.
func ocrFixes(text, lang string) string {
	en := baseLang(lang) == "en"
	return eachLine(text, func(l string) string {
		return eachWord(l, func(w string) string {
			w = zeroToO(w)
			if en {
				if w == "|" {
					return "I"
				}
				if enLForI[strings.ReplaceAll(w, "’", "'")] {
					return "I" + w[1:]
				}
			}
			return w
		})
	})
}

// zeroToO turns each 0 that sits between two letters into O, or o when both
// letters are lower case, as in "G0D" read for "GOD" or "c0ld" for "cold".
func zeroToO(w string) string {
	if strings.IndexByte(w, '0') < 0 {
		return w
	}
	b := []byte(w)
	for i := 1; i+1 < len(b); i++ {
		if b[i] != '0' {
			continue
		}
		p, _ := utf8.DecodeLastRune(b[:i])
		n, _ := utf8.DecodeRune(b[i+1:])
		if !unicode.IsLetter(p) || !unicode.IsLetter(n) {
			continue
		}
		b[i] = 'O'
		if unicode.IsLower(p) && unicode.IsLower(n) {
			b[i] = 'o'
		}
	}
	return string(b)
}

// commonFixes tidies spacing, punctuation, dialogue dashes and ellipses.
func commonFixes(text, lang string) string {
	// French puts a space before ! ? ; and :, so those spaces stay.
	fr := baseLang(lang) == "fr"
	return eachLine(text, func(l string) string {
		l = collapseSpaces(strings.ReplaceAll(l, "…", "..."))
		l = collapseDots(l)
		l = fixDash(l)
		l = spaceBeforePunct(l, fr)
		return spaceAfterComma(l)
	})
}

// collapseSpaces turns runs of spaces and tabs into one space and trims line.
func collapseSpaces(line string) string {
	var sb strings.Builder
	space := false
	for _, r := range strings.TrimSpace(line) {
		if r == ' ' || r == '\t' {
			space = true
			continue
		}
		if space {
			sb.WriteByte(' ')
			space = false
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// collapseDots turns four or more dots in a row into an ellipsis of three.
func collapseDots(line string) string {
	if !strings.Contains(line, "....") {
		return line
	}
	var sb strings.Builder
	run := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '.' {
			run++
			if run <= 3 {
				sb.WriteByte('.')
			}
			continue
		}
		run = 0
		sb.WriteByte(line[i])
	}
	return sb.String()
}

// fixDash writes a dialogue dash at the start of a line as "- ". A hyphen,
// double hyphen or long dash counts as one only when a capital letter or an
// opening quote follows, so "--and then" carrying on a cut-off line stays.
func fixDash(line string) string {
	lead := leadingTags(line)
	t := line[len(lead):]
	for _, d := range []string{"--", "–", "—", "-"} {
		rest, ok := strings.CutPrefix(t, d)
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, " ")
		r, _ := utf8.DecodeRuneInString(rest)
		if unicode.IsUpper(r) || r == '"' || r == '\'' || r == '¿' || r == '¡' {
			return lead + "- " + rest
		}
		break
	}
	return line
}

// spaceBeforePunct drops a space before , ! ? ; : and before a full stop that
// is not the start of an ellipsis. With keepFrench the space before ! ? ; :
// stays.
func spaceBeforePunct(line string, keepFrench bool) string {
	if !strings.Contains(line, " ") {
		return line
	}
	var sb strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' && i+1 < len(line) && i > 0 {
			switch c := line[i+1]; {
			case c == ',':
				continue
			case c == '.' && (i+2 >= len(line) || line[i+2] != '.'):
				continue
			case !keepFrench && strings.IndexByte("!?;:", c) >= 0:
				continue
			}
		}
		sb.WriteByte(line[i])
	}
	return sb.String()
}

// spaceAfterComma adds the missing space in "word,word". Numbers such as
// 1,000 keep theirs.
func spaceAfterComma(line string) string {
	if !strings.Contains(line, ",") {
		return line
	}
	var sb strings.Builder
	for i := 0; i < len(line); i++ {
		sb.WriteByte(line[i])
		if line[i] != ',' || i == 0 || i+1 >= len(line) {
			continue
		}
		p, _ := utf8.DecodeLastRuneInString(line[:i])
		n, _ := utf8.DecodeRuneInString(line[i+1:])
		if unicode.IsLetter(p) && unicode.IsLetter(n) {
			sb.WriteByte(' ')
		}
	}
	return sb.String()
}

// fixUppercase turns lines written all in capitals into sentence case: lower
// case, with a capital at the start of each sentence and, in English, for the
// word I. A line keeps the sentence state of the line before it, so the
// second line of a sentence stays in lower case.
func fixUppercase(text, lang string) string {
	c := unicode.SpecialCase(nil)
	if baseLang(lang) == "tr" || baseLang(lang) == "az" {
		c = unicode.TurkishCase
	}
	en := baseLang(lang) == "en"
	lines := strings.Split(text, "\n")
	capNext := true
	changed := false
	for i, l := range lines {
		if !shouty(l) {
			capNext = endsSentence(l)
			continue
		}
		lines[i], capNext = sentenceCase(l, c, capNext)
		if en {
			lines[i] = eachWord(lines[i], func(w string) string {
				if enLowerI[strings.ReplaceAll(w, "’", "'")] {
					return "I" + w[1:]
				}
				return w
			})
		}
		changed = true
	}
	if !changed {
		return text
	}
	return strings.Join(lines, "\n")
}

// shouty reports whether line has at least four cased letters outside its
// tags and none in lower case. Short lines such as "FBI" or "OK" stay.
func shouty(line string) bool {
	upper := 0
	for _, r := range stripTags(line) {
		switch {
		case unicode.IsLower(r):
			return false
		case unicode.IsUpper(r):
			upper++
		}
	}
	return upper >= 4
}

// sentenceCase lowers line and raises the first letter of each sentence,
// starting with a capital when capNext is set. Tags are copied as they are.
// It returns the line and whether the next line starts a sentence.
func sentenceCase(line string, c unicode.SpecialCase, capNext bool) (string, bool) {
	var sb strings.Builder
	for i := 0; i < len(line); {
		if n := tagLen(line[i:]); n > 0 {
			sb.WriteString(line[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		i += size
		switch {
		case unicode.IsLetter(r):
			if capNext {
				r = c.ToUpper(r)
				capNext = false
			} else {
				r = c.ToLower(r)
			}
		case unicode.IsDigit(r):
			capNext = false
		case r == '.' || r == '!' || r == '?':
			capNext = true
		}
		sb.WriteRune(r)
	}
	return sb.String(), capNext
}

// endsSentence reports whether line ends a sentence, or shows nothing.
func endsSentence(line string) bool {
	t := strings.TrimRight(stripTags(line), " \t\"'")
	if t == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(t)
	return r == '.' || r == '!' || r == '?' || r == '…'
}
