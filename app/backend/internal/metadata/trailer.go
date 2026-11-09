package metadata

import "strings"

// PickTrailer chooses the video a title's trailer plays: a Trailer before a
// Teaser, official before not, the person's language before English before
// any other, keeping TMDB's own order otherwise.
func PickTrailer(videos []DetailVideo, lang string) (DetailVideo, bool) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	score := func(v DetailVideo) int {
		s := 0
		if v.Type == "Trailer" {
			s += 100
		} else if v.Type != "Teaser" {
			return -1
		}
		if v.Official {
			s += 10
		}
		switch l := strings.ToLower(v.Language); {
		case lang != "" && l == lang:
			s += 5
		case l == "en" || l == "":
			s += 2
		}
		return s
	}
	best, bestScore := DetailVideo{}, -1
	for _, v := range videos {
		if !youTubeKey.MatchString(v.Key) {
			continue
		}
		if sc := score(v); sc > bestScore {
			best, bestScore = v, sc
		}
	}
	return best, bestScore >= 0
}
