package suggest

import (
	"strconv"
	"strings"
	"time"
)

// Freshness kinds, listed highest priority first in FreshOrder.
const (
	FreshNewEpisode    = "new_episode"
	FreshNewSeason     = "new_season"
	FreshRecentlyAdded = "recently_added"
	FreshComing        = "coming"
	FreshSeasonComing  = "season_coming"
	FreshNewRelease    = "new_release"
)

const day = 24 * time.Hour

// FreshOrder is the rule priority: a card shows the first rule that holds.
var FreshOrder = []string{FreshNewEpisode, FreshNewSeason, FreshRecentlyAdded, FreshComing, FreshSeasonComing, FreshNewRelease}

// RowSuppress maps a row type to the strips its cards never show, so a card
// does not repeat what its row already says. A row not listed suppresses nothing.
var RowSuppress = map[string][]string{
	TypeBest:        {FreshNewRelease},
	TypeNewServices: {FreshNewRelease},
	"fresh":         {FreshNewRelease},
	"new_from":      {FreshNewRelease},
	"just_arrived":  {FreshRecentlyAdded},
	"recent":        {FreshRecentlyAdded},
	"coming_soon":   {FreshComing, FreshSeasonComing},
	TypeCinemas:     {FreshComing},
	TypeReturning:   {FreshSeasonComing},
	"upcoming":      {FreshComing, FreshSeasonComing},
	"new_episodes":  {FreshNewEpisode},
	"new_seasons":   {FreshNewSeason},
}

// Fresh is the one freshness strip on a card.
type Fresh struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Rank int    `json:"rank,omitempty"`
}

// FreshFacts is what the rules read about one card. Zero times mean unknown.
type FreshFacts struct {
	Kind      string // movie, show, album, artist
	InLibrary bool
	// Added is when the title landed in the Library.
	Added time.Time
	// LastEpisode is the newest episode aired so far.
	LastEpisode time.Time
	// SeasonStart is when the newest started season (number above 1) began.
	SeasonStart time.Time
	// NextSeason is the soonest announced season still to come.
	NextSeason       time.Time
	NextSeasonNumber int
	// Released is the movie's release, or the show's first air date.
	Released time.Time
	// Music cards only use the recently added and new release rules.
	Music bool
}

// Suppressed reports whether row type row never shows strip kind k.
func Suppressed(row, k string) bool {
	for _, s := range RowSuppress[row] {
		if s == k {
			return true
		}
	}
	return false
}

// PickFresh returns the card's strip, or nil when no rule holds.
func PickFresh(f FreshFacts, now time.Time) *Fresh { return PickFreshIn(f, now, "") }

// PickFreshIn returns the first strip that holds and that a row of type row
// does not suppress, or nil.
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

// ParseDay reads a stored date or time, either a full timestamp or YYYY-MM-DD.
// It returns the zero time when the text is not a date.
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

func midnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// daysAgo counts whole calendar days from t to now, negative when t is still
// ahead. ok is false for an unknown time.
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
	case FreshComing:
		// Only releases within a year count as coming; past that is a placeholder date.
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
