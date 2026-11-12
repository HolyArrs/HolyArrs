package arrapi

import (
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// looseID is an id a client may send as a number or as number text, as
// Pulsarr sends tag ids and sometimes the Pick rules id. Text that is not
// a whole number reads as -1, so the add can refuse that one field
// instead of the whole body.
type looseID int64

// UnmarshalJSON reads a number, number text or null.
func (v *looseID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*v = 0
		return nil
	}
	if q, err := strconv.Unquote(s); err == nil {
		s = strings.TrimSpace(q)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		*v = -1
		return nil
	}
	*v = looseID(n)
	return nil
}

// tagIDs keeps the ids that can name a tag.
func tagIDs(in []looseID) []int64 {
	var out []int64
	for _, id := range in {
		if id > 0 {
			out = append(out, int64(id))
		}
	}
	return out
}

// recentWindow is how far back the old apps' "recent" choice reaches.
const recentWindow = 90 * 24 * time.Hour

// seasonPlan applies one of the old apps' season choices to a show add.
// The choice wins over a season list, as in the old apps; no choice, or
// "unknown", keeps the season list (or every season when none is sent).
// The fits are the closest HolyArrs adds:
//
//   - all and missing: every season (the search only grabs what is
//     missing anyway);
//   - future: only episodes still to air;
//   - recent: every season, episodes that aired in the last 90 days or
//     are still to air kept;
//   - existing: a show just added has no files yet, so nothing is kept;
//   - pilot: the first season with only its first episode kept;
//   - firstSeason: the first season;
//   - lastSeason and latestSeason: the latest season;
//   - monitorSpecials and unmonitorSpecials: every season, Specials on
//     or off;
//   - none and skip: nothing kept and no search, even if asked for.
//
// A word it does not know counts as no choice. It reports false for no
// choice. noSearch is true when the search must not run.
func seasonPlan(monitor string, now time.Time, opt *shows.ShowAddOptions) (given, noSearch bool) {
	on, off := true, false
	switch strings.ToLower(strings.TrimSpace(monitor)) {
	case "all", "missing":
		opt.Seasons = library.SeasonsAll
	case "future":
		opt.Seasons = library.SeasonsFuture
	case "recent":
		opt.Seasons = library.SeasonsAll
		cut := now.Add(-recentWindow)
		opt.KeepEpisode = func(_, _ int, aired *time.Time) bool { return aired == nil || aired.After(cut) }
	case "existing":
		opt.Seasons = library.SeasonsNone
	case "pilot":
		opt.FirstSeason = true
		opt.KeepEpisode = func(_, episode int, _ *time.Time) bool { return episode == 1 }
	case "firstseason":
		opt.FirstSeason = true
	case "lastseason", "latestseason":
		opt.Seasons = library.SeasonsLatest
	case "monitorspecials":
		opt.Seasons, opt.Specials = library.SeasonsAll, &on
	case "unmonitorspecials":
		opt.Seasons, opt.Specials = library.SeasonsAll, &off
	case "none", "skip":
		opt.Seasons = library.SeasonsNone
		return true, true
	default:
		return false, false
	}
	return true, false
}
