package releasename

import "strings"

// titleWordTags are tag words that are also everyday title words ("The
// Cam", "A Complete Unknown", "Dual"). One of them stays in the title when
// the next word is a year or a plain word, as the old apps read it.
var titleWordTags = setOf("cam", "ts", "complete", "web", "dvd", "dual", "real", "extras", "trailer", "sample",
	"password", "internal", "proper", "dubbed")

func titleTag(toks []string, i int) bool {
	if !titleWordTags[strings.ToLower(toks[i])] || i+1 >= len(toks) {
		return false
	}
	next := strings.ToLower(toks[i+1])
	return reYear.MatchString(next) && i+2 < len(toks) || plainWord(next) && !reNumber(next)
}

// plainWord says a lower-case word is neither a tag, a language nor a
// number, so it reads as part of a title or a note.
func plainWord(t string) bool {
	_, lang := moreLangWords[t]
	return !lang && !isTag(t) && !reNumber(t) && t != "dub" && t != "audio"
}
