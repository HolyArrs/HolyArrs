package sources

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func textQuery(q Query) string {
	// Apostrophes, and dots between single letters, go without a space
	// (Schitt's -> Schitts, S.W.A.T. -> SWAT), as the older apps send show
	// and movie names: most sites find nothing when the term keeps them.
	// Other punctuation, other dots and stars too, splits words (M*A*S*H ->
	// M A S H, Jackass 2.5 -> Jackass 2 5, D.Gray-man -> D Gray man).
	words := splitWords(releasename.JoinDottedLetters(q.Title))
	if q.Year > 0 {
		words = append(words, strconv.Itoa(q.Year))
	}
	return strings.Join(words, " ")
}

// musicText is the search text for an artist and album: as the older music
// app sends it, a dot is a gap (R.E.M. -> R E M).
func musicText(s string) string { return strings.Join(splitWords(s), " ") }

// splitWords drops apostrophes and splits on every other mark.
func splitWords(s string) []string {
	s = strings.NewReplacer("'", "", "’", "", "‘", "", "`", "").Replace(s)
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// nameWords are a title's or a release name's words for the id-answer
// check, lower case, with one-letter words in a row joined, so "P.S. I Love
// You" and "P.S.I.Love.You" read alike.
func nameWords(s string) []string {
	return strings.Fields(releasename.JoinLetters(strings.ToLower(strings.Join(splitWords(s), " "))))
}

// namesAll says whether a release name holds every word of the title, read
// with one-letter words joined or as written ("Plan.B.H.264"), or the
// title's letters in a row however the release spaces them ("PS.I.Love").
func namesAll(release string, words []string) bool {
	rel := nameWords(release)
	lists := [][]string{rel}
	if len(words) > 1 {
		// As written only for a title of several words: a one-letter title
		// ("M") would be found in any letters written apart ("House.M.D").
		lists = append(lists, strings.Fields(strings.ToLower(strings.Join(splitWords(release), " "))))
	}
	for _, list := range lists {
		got := map[string]bool{}
		for _, w := range list {
			got[w] = true
		}
		all := true
		for _, w := range words {
			all = all && got[w]
		}
		if all {
			return true
		}
	}
	key := strings.Join(words, "")
	for i := range rel {
		run := ""
		for _, w := range rel[i:] {
			if run += w; len(run) >= len(key) {
				break
			}
		}
		if run == key {
			return true
		}
	}
	return false
}
