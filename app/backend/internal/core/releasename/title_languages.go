package releasename

import (
	"regexp"
	"slices"
	"strings"
)

var (
	reNameWord = regexp.MustCompile(`[\p{L}\p{N}']+`)
	reNameGaps = regexp.MustCompile(`[ ._-]{2,}`)
)

// WithoutTitleWords drops the languages in langs that come only from the
// words of a matched title, like the old apps do: "Show.S01E01.The.German.Job"
// with the episode "The German Job" reads no German. Each title of two or
// more words is cut from the name once, the name is read again, and a
// language the name held stays only if that reading still finds it;
// languages from elsewhere (a Source's own list) always stay. One-word titles are
// left alone, so an episode called "French" keeps a real FRENCH tag.
func WithoutTitleWords(name string, h Hints, langs []string, titles ...string) []string {
	if len(langs) == 0 {
		return langs
	}
	locs := reNameWord.FindAllStringIndex(name, -1)
	cut := false
	keep := make([]bool, len(locs))
	for i := range keep {
		keep[i] = true
	}
	for _, t := range titles {
		words := reNameWord.FindAllString(strings.ToLower(t), -1)
		if len(words) < 2 {
			continue
		}
	next:
		for i := 0; i+len(words) <= len(locs); i++ {
			for j, w := range words {
				l := locs[i+j]
				if !keep[i+j] || strings.ToLower(name[l[0]:l[1]]) != w {
					continue next
				}
			}
			for j := range words {
				keep[i+j] = false
			}
			cut = true
			break
		}
	}
	if !cut {
		return langs
	}
	var b strings.Builder
	last := 0
	for i, l := range locs {
		if keep[i] {
			continue
		}
		b.WriteString(name[last:l[0]])
		last = l[1]
	}
	b.WriteString(name[last:])
	rest := reNameGaps.ReplaceAllStringFunc(b.String(), func(s string) string { return s[:1] })
	read, again := Parse(name, h).Languages, Parse(rest, h).Languages
	out := make([]string, 0, len(langs))
	for _, l := range langs {
		// Languages the name does not hold (a Source sent them) stay.
		if !slices.Contains(read, l) || slices.Contains(again, l) {
			out = append(out, l)
		}
	}
	return out
}
