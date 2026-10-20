package explore

import (
	"strconv"
	"strings"
	"time"
)

// Freshness kinds, highest priority first (FreshOrder).
const (
	FreshNewEpisode    = "new_episode"
	FreshNewSeason     = "new_season"
	FreshRecentlyAdded = "recently_added"
	FreshTop10         = "top10"
	FreshComing        = "coming"
	FreshSeasonComing  = "season_coming"
	FreshNewRelease    = "new_release"
)

// FreshOrder is the priority: a card shows the first rule that holds.
var FreshOrder = []string{FreshNewEpisode, FreshNewSeason, FreshRecentlyAdded, FreshTop10, FreshComing, FreshSeasonComing, FreshNewRelease}

// Fresh is the one freshness strip on a card.
type Fresh struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Rank int    `json:"rank,omitempty"`
}

// FreshFacts is what the rules read about one card. Zero times are unknown.
type FreshFacts struct {
	Kind      string // movie, show, album, artist
	InLibrary bool
	// Added is when the title landed in the Library.
	Added time.Time
	// LastEpisode is the newest episode aired so far (shows in the Library).
	LastEpisode time.Time
	// SeasonStart is when the newest started season (number > 1) began.
	SeasonStart time.Time
	// NextSeason is the soonest announced season still to come.
	NextSeason       time.Time
	NextSeasonNumber int
	// Released is the movie's release, or the show's first air date.
	Released time.Time
	// Top10Rank is the rank in this week's Top 10 for the region, 0 if not in it.
	Top10Rank int
	// Music cards only use Recently added and New release.
	Music bool
}

const day = 24 * time.Hour

func midnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// daysAgo is whole days from t to now (negative when t is still to come);
// ok is false for an unknown time.
func daysAgo(t, now time.Time) (int, bool) {
	if t.IsZero() {
		return 0, false
	}
	return int(midnight(now).Sub(midnight(t)) / day), true
}

func within(t, now time.Time, days int) bool {
	d, ok := daysAgo(t, now)
	return ok && d >= 0 && d < days
}

// RowSuppress maps a row type to the strips its cards never show: a card
// never repeats what its row already says (no Top 10 tag in Top 10, no New
// Release in Best new releases). Keys are Explore, discover and music row
// types; a row not listed suppresses nothing.
var RowSuppress = map[string][]string{
	"top10":         {FreshTop10},
	TypeBest:        {FreshNewRelease},
	TypeNewServices: {FreshNewRelease},
	"fresh":         {FreshNewRelease}, // music: New this week
	"new_from":      {FreshNewRelease}, // music: New from artists you have
	"just_arrived":  {FreshRecentlyAdded},
	"recent":        {FreshRecentlyAdded},
	"coming_soon":   {FreshComing, FreshSeasonComing},
	TypeCinemas:     {FreshComing},
	TypeReturning:   {FreshSeasonComing},
	"upcoming":      {FreshComing, FreshSeasonComing},
	"new_episodes":  {FreshNewEpisode},
	"new_seasons":   {FreshNewSeason},
}

// Suppressed says row type row never shows strip kind k.
func Suppressed(row, k string) bool {
	for _, s := range RowSuppress[row] {
		if s == k {
			return true
		}
	}
	return false
}

// PickFresh is the card's freshness strip, or nil when no rule holds.
func PickFresh(f FreshFacts, now time.Time) *Fresh { return PickFreshIn(f, now, "") }

// PickFreshIn is the card's one strip in a row of type row: the first rule
// that holds and that the row does not suppress.
func PickFreshIn(f FreshFacts, now time.Time, row string) *Fresh {
	for _, k := range FreshOrder {
		if f.Music && k != FreshRecentlyAdded && k != FreshNewRelease {
			continue
		}
		if Suppressed(row, k) {
			continue
		}
		if b := freshRule(k, f, now); b != nil {
			return b
		}
	}
	return nil
}

func freshRule(k string, f FreshFacts, now time.Time) *Fresh {
	show := f.Kind == "show"
	switch k {
	case FreshNewEpisode:
		if show && f.InLibrary && within(f.LastEpisode, now, 7) {
			return &Fresh{Kind: k, Text: "New Episode"}
		}
	case FreshNewSeason:
		if show && within(f.SeasonStart, now, 14) {
			return &Fresh{Kind: k, Text: "New Season"}
		}
	case FreshRecentlyAdded:
		if f.InLibrary && within(f.Added, now, 7) {
			return &Fresh{Kind: k, Text: "Recently Added"}
		}
	case FreshTop10:
		if f.Top10Rank >= 1 && f.Top10Rank <= 10 {
			return &Fresh{Kind: k, Text: "Trending", Rank: f.Top10Rank}
		}
	case FreshComing:
		if d, ok := daysAgo(f.Released, now); ok && d < 0 && d >= -365 {
			if d >= -7 {
				return &Fresh{Kind: k, Text: "Coming " + f.Released.UTC().Weekday().String()}
			}
			return &Fresh{Kind: k, Text: "Premieres " + f.Released.UTC().Format("Jan 2")}
		}
	case FreshSeasonComing:
		if d, ok := daysAgo(f.NextSeason, now); show && ok && d < 0 && f.NextSeasonNumber > 0 {
			return &Fresh{Kind: k, Text: "Season " + strconv.Itoa(f.NextSeasonNumber) + " coming"}
		}
	case FreshNewRelease:
		if f.Kind != "show" && f.Kind != "artist" && within(f.Released, now, 30) {
			return &Fresh{Kind: k, Text: "New Release"}
		}
	}
	return nil
}

// ParseDay reads a stored date or time: full timestamps or YYYY-MM-DD.
func ParseDay(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, l := range []string{"2006-01-02T15:04:05.000000000Z", time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return t
		}
	}
	return time.Time{}
}
