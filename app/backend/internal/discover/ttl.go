package discover

import (
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Titles are the row headings the app may show.
var Titles = map[string]string{
	"trending": "Trending this week", "popular": "Popular", "upcoming": "Coming soon",
	"similar": "Because you have it", "genre": "More in this genre", "services": "On your services", "best": "Best new releases",
}

// TTL is how long a row from TMDB is kept.
var TTL = map[string]time.Duration{
	"trending": 6 * time.Hour, "popular": 12 * time.Hour, "upcoming": 12 * time.Hour,
	"similar": 24 * time.Hour, "genre": 24 * time.Hour, "details": 24 * time.Hour, "services": 12 * time.Hour, "best": 24 * time.Hour,
}

// CollectionRetry is how long details whose collection could not be read
// are kept before they are asked for again.
const CollectionRetry = 5 * time.Minute

// DetailsTTL is how long a details answer is kept: a day, or a few minutes
// when its movie collection could not be read.
func DetailsTTL(e Entry) time.Duration {
	if e.Details != nil && e.Details.CollectionMissed {
		return CollectionRetry
	}
	if e.Details != nil && peopleWithoutIDs(e.Details.People) {
		return 0
	}
	if e.Details != nil && len(e.Details.Videos) == 0 {
		// No trailer yet: asked again soon (the server may find a season's).
		return 15 * time.Minute
	}
	return TTL["details"]
}

// peopleWithoutIDs is true for details cached before people carried their
// TMDB ids: credits with no id at all, so the page is read again.
func peopleWithoutIDs(people []metadata.DetailPerson) bool {
	for _, p := range people {
		if p.ID != "" {
			return false
		}
	}
	return len(people) > 0
}

// QualityWord names the best picture the Library has: 4K, HD, or nothing.
func QualityWord(resolution int64) string {
	switch {
	case resolution >= 2160:
		return "4K"
	case resolution >= 720:
		return "HD"
	}
	return ""
}
