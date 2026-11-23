package library

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

var packReleaseYear = regexp.MustCompile(`^(.+?)[ ._-]+((?:19|20)\d{2})[ ._-]+(.+)$`)
var packLeadingEdition = regexp.MustCompile(`(?i)^(?:extended director'?s cut|director'?s cut|special edition|extended(?: edition| cut)?|unrated|uncut|theatrical cut)\s+`)

// Read structural release evidence even when title matching cannot parse the
// person/edition annotations. Require one year and a technical release tail.
func packReleaseFacts(name string) (string, int) {
	m := packReleaseYear.FindStringSubmatch(name)
	if m == nil || titleYearToken.MatchString(m[1]) || titleYearToken.MatchString(m[3]) || !folderReleaseWords.MatchString(packBrackets.Replace(m[3])) {
		return "", 0
	}
	y, _ := strconv.Atoi(m[2])
	return m[1], y
}

func numberedPackLookup(rel, name string) string {
	if !collectionPackPath(rel) || reTMDBTag.MatchString(name) {
		return name
	}
	title, year := packReleaseFacts(name)
	numbered := packOrdinal.FindStringSubmatch(title)
	if year == 0 || numbered == nil {
		return name
	}
	words := strings.Fields(strings.ToLower(numbered[1]))
	if len(words) > 1 && words[0] == "the" {
		words = words[1:]
	}
	confirmed := false
	for dir := path.Dir(rel); dir != "." && dir != "/"; dir = path.Dir(dir) {
		parent := strings.TrimPrefix(strings.ToLower(strings.ReplaceAll(path.Base(dir), ".", " ")), "the ")
		if len(words) > 0 && matchingCollectionName(parent) && strings.HasPrefix(parent, words[0]+" ") {
			confirmed = true
			break
		}
	}
	if !confirmed {
		return name
	}
	title = numbered[1]
	if main, annotation, ok := strings.Cut(title, " - "); ok {
		for i := 0; i < 3; i++ {
			clean := packLeadingEdition.ReplaceAllString(annotation, "")
			if clean == annotation {
				break
			}
			annotation = clean
		}
		// Keep names and unknown words for metadata/credit verification.
		title = strings.TrimSpace(main + " " + annotation)
	}
	return title + " (" + strconv.Itoa(year) + ")"
}
