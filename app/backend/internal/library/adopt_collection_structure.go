package library

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

var collectionReleaseYear = regexp.MustCompile(`^(.+?)[ ._-]+((?:19|20)\d{2})[ ._-]+(.+)$`)
var collectionYearFirst = regexp.MustCompile(`(?i)^((?:19|20)\d{2})[ ._-]+(.+?)[ ._-]+\d{3,4}x\d{3,4}[ ._-]+(?:BDRip|BRRip|BluRay|WEBRip|WEB-DL)\b`)

// Read structural release evidence even when title matching cannot parse the
// person/edition annotations. Require one year and a technical release tail.
func collectionReleaseFacts(name string) (string, int) {
	if m := collectionYearFirst.FindStringSubmatch(name); m != nil && !collectionYearToken.MatchString(m[2]) {
		y, _ := strconv.Atoi(m[1])
		return strings.NewReplacer(".", " ", "_", " ").Replace(m[2]), y
	}
	m := collectionReleaseYear.FindStringSubmatch(name)
	if m == nil || collectionYearToken.MatchString(m[1]) || collectionYearToken.MatchString(m[3]) || !collectionReleaseWords.MatchString(collectionBrackets.Replace(m[3])) {
		return "", 0
	}
	y, _ := strconv.Atoi(m[2])
	return m[1], y
}

// Ambiguous pack wrappers must not silently adopt their largest video.
func possibleCollectionPack(folder string, videos []AdoptVideo) bool {
	if !collectionName(path.Base(folder)) || len(videos) < 2 {
		return false
	}
	identities := map[string]bool{}
	unknown := false
	hasDisc, hasLoose := false, false
	for _, v := range videos {
		extra, disc := false, false
		parts := strings.Split(strings.TrimPrefix(v.Path, folder+"/"), "/")
		for _, part := range parts[:len(parts)-1] {
			if reDiscFolder.MatchString(strings.ToLower(strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(part))) {
				disc = true
			}
			if partFolder(part) && !disc {
				extra = true
			}
		}
		if extra {
			continue
		}
		if disc {
			hasDisc = true
		} else {
			hasLoose = true
		}
		name, year := FolderName(candidateLookup(v.Path))
		if year == 0 {
			name, year = collectionReleaseFacts(candidateLookup(v.Path))
		}
		if year == 0 {
			unknown = true
			continue
		}
		identities[strings.ToLower(name)+"/"+strconv.Itoa(year)] = true
	}
	if hasDisc && !hasLoose && len(identities) <= 1 {
		return false
	}
	return unknown || len(identities) > 1
}

var collectionYearToken = regexp.MustCompile(`(?:^|[ ._-])(?:18|19|20)\d{2}(?:$|[ ._-])`)
var collectionBrackets = strings.NewReplacer("[", " ", "]", " ")
var collectionReleaseWords = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:\d{3,4}[pi]|[48]k|blu-?ray|web-?dl|webrip|b[dr]rip|dvdrip|x26[45]|h26[45]|hevc|av1|hdr10\+?|hdr|sdr|dv|aac|dts|atmos)(?:$|[\s._-])`)
