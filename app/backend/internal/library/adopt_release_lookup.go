package library

import (
	"path"
	"regexp"
	"strings"
)

var yearFirstRelease = regexp.MustCompile(`(?i)^((?:19|20)\d{2})[ ._-]+(.+?)[ ._-]+\d{3,4}x\d{3,4}[ ._-]+(?:BDRip|BRRip|BluRay|WEBRip|WEB-DL)\b`)
var titleYearToken = regexp.MustCompile(`(?:^|[ ._-])(?:18|19|20)\d{2}(?:$|[ ._-])`)
var releaseEdition = regexp.MustCompile(`(?i)\s+(?:director'?s cut|special edition|extended(?: edition| cut)?|unrated|uncut|theatrical cut)$`)
var releaseAcronym = regexp.MustCompile(`\s+([A-Z]{2,6})[0-9]{0,2}$`)

// yearFirstLookup requires a resolution and release source, and refuses a
// second title/year token. Bare numeric titles are not treated as dates.
func yearFirstLookup(name string) string {
	m := yearFirstRelease.FindStringSubmatch(name)
	if m == nil || titleYearToken.MatchString(m[2]) || reTMDBTag.MatchString(name) {
		return name
	}
	return strings.TrimSpace(strings.ReplaceAll(m[2], ".", " ")) + " (" + m[1] + ")"
}

// cleanPackEdition operates only on an annotated collection-pack title. It
// removes suffixes, never words in the middle of a title or sequel numbers.
func cleanPackEdition(title string) string {
	for i := 0; i < 3; i++ {
		clean := releaseEdition.ReplaceAllString(title, "")
		if clean == title || strings.TrimSpace(clean) == "" {
			break
		}
		title = clean
	}
	if m := releaseAcronym.FindStringSubmatchIndex(title); m != nil {
		base, acronym := title[:m[0]], title[m[2]:m[3]]
		words := strings.Fields(base)
		if len(words) >= len(acronym) {
			var initials strings.Builder
			for _, w := range words[:len(acronym)] {
				initials.WriteString(strings.ToUpper(w[:1]))
			}
			if initials.String() == acronym {
				title = base
			}
		}
	}
	return title
}

func matchingLookup(rel string) string {
	n := path.Base(rel)
	if videoExt[strings.ToLower(path.Ext(n))] {
		n = strings.TrimSuffix(n, path.Ext(n))
	}
	return numberedPackLookup(rel, packLookup(rel, yearFirstLookup(n)))
}

var matchingCollectionYears = regexp.MustCompile(`(?:19|20)\d{2}\s*[-–]\s*(?:19|20)\d{2}`)
var matchingCollectionWords = regexp.MustCompile(`(?i)\b(?:collection|trilogy|quadrilogy|anthology)\b`)

func matchingCollectionName(name string) bool {
	return matchingCollectionYears.MatchString(name) || strings.Contains(strings.ToLower(name), "collection") || matchingCollectionWords.MatchString(strings.ReplaceAll(name, "_", " "))
}
