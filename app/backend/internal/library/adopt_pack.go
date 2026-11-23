package library

import (
	"path"
	"regexp"
	"strings"
)

var packGenreYear = regexp.MustCompile(`(?i)^(.+?)\s+-\s+(?:horror|sci-fi|thriller|action|comedy|drama|adventure|fantasy|animation)\s+((?:19|20)\d{2})(?:\s.*)?$`)
var packOrdinal = regexp.MustCompile(`^(?:0[1-9]|[1-9][0-9])\s+(.+)$`)
var packBrackets = strings.NewReplacer("[", " ", "]", " ")

// packLookup removes release annotations from collection-pack lookup text.
// It never changes paths. A numbered prefix needs both release annotations
// and a collection ancestor named for the remaining title's first word.
// Generic folders such as "Collection" do not justify dropping title numbers.
func packLookup(rel, name string) string {
	m := packGenreYear.FindStringSubmatch(name)
	if m == nil || reTMDBTag.MatchString(name) || !folderReleaseWords.MatchString(packBrackets.Replace(name)) {
		return name
	}
	title := m[1]
	if numbered := packOrdinal.FindStringSubmatch(title); numbered != nil {
		words := strings.Fields(strings.ToLower(numbered[1]))
		if len(words) > 1 && words[0] == "the" {
			words = words[1:]
		}
		for dir := path.Dir(rel); dir != "." && dir != "/"; dir = path.Dir(dir) {
			parent := strings.ToLower(strings.ReplaceAll(path.Base(dir), ".", " "))
			if len(words) > 0 && strings.Contains(parent, "collection") && strings.HasPrefix(parent, words[0]+" ") {
				title = numbered[1]
				break
			}
		}
	}
	if collectionPackPath(rel) {
		title = cleanPackEdition(title)
	}
	return title + " (" + m[2] + ")"
}

func collectionPackPath(rel string) bool {
	for dir := path.Dir(rel); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if matchingCollectionName(path.Base(dir)) {
			return true
		}
	}
	return false
}
