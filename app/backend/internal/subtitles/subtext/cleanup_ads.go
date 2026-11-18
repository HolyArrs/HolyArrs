package subtext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// adWindow is how many cues at each end of a subtitle are checked for ads and
// credits. Sites put them first or last, and checking only there keeps a line
// of dialogue that happens to name a website.
const adWindow = 4

// adPass is the score at which a cue counts as an ad or a credit.
const adPass = 2

// adLongCue is the length in letters past which a cue is more likely
// dialogue than a credit, and loses a point.
const adLongCue = 120

// adCues returns the indexes of the cues near the start and the end of cues
// that score as ads or credits.
func adCues(cues []Cue, lang string) map[int]bool {
	hits := map[int]bool{}
	words := adWords(lang)
	check := func(i int) {
		if !hits[i] && adScore(cues[i].Text, words) >= adPass {
			hits[i] = true
		}
	}
	for i := 0; i < min(adWindow, len(cues)); i++ {
		check(i)
	}
	for i := max(len(cues)-adWindow, 0); i < len(cues); i++ {
		check(i)
	}
	return hits
}

// adScore scores one cue: two points for a web address and two for each ad
// or credit phrase, less one when the cue is long.
func adScore(text string, words []string) int {
	p := strings.ToLower(strings.Join(strings.Fields(stripTags(text)), " "))
	score := 0
	if hasWebAddress(p) {
		score += 2
	}
	for _, w := range words {
		if containsWord(p, w) {
			score += 2
		}
	}
	if utf8.RuneCountInString(p) > adLongCue {
		score--
	}
	return score
}

// adWords returns the phrases checked for lang: the ones for every language
// and the ones for lang or its base, such as pt for pt-br.
func adWords(lang string) []string {
	out := append([]string{}, adAnyLanguage...)
	out = append(out, adByLanguage[strings.ToLower(lang)]...)
	if b := baseLang(lang); b != strings.ToLower(lang) {
		out = append(out, adByLanguage[b]...)
	}
	return out
}

// containsWord reports whether w appears in s with no letter or digit right
// before or after it, so "yts" is found in "yts.mx" but not in "nyts".
func containsWord(s, w string) bool {
	for off := 0; off < len(s); {
		i := strings.Index(s[off:], w)
		if i < 0 {
			return false
		}
		i += off
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		after, _ := utf8.DecodeRuneInString(s[i+len(w):])
		if !wordChar(before) && !wordChar(after) {
			return true
		}
		off = i + 1
	}
	return false
}

func wordChar(r rune) bool { return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r)) }

// hasWebAddress reports whether lower-case text holds something that reads as
// a web address: http://, www. or a name ending in a common top-level domain.
func hasWebAddress(text string) bool {
	if !strings.Contains(text, ".") {
		return false
	}
	for _, f := range strings.Fields(text) {
		f = strings.Trim(f, "\"'()[]<>,.!?:;")
		if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") || strings.HasPrefix(f, "www.") {
			return true
		}
		host, _, _ := strings.Cut(f, "/")
		labels := strings.Split(host, ".")
		if len(labels) < 2 || !webTLDs[labels[len(labels)-1]] {
			continue
		}
		ok := true
		for _, l := range labels {
			if l == "" || strings.Trim(l, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
