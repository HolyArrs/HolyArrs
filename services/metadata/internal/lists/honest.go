package lists

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// Honest titles, dates and matches (2026-10-24 audit): edition junk off the
// titles, each album's first release date from MusicBrainz, the original
// release group over compilations, live albums and hits sets, and a song
// charting now never shown as a years-old album it happens to share
// artwork with.

var (
	junkParen = regexp.MustCompile(`(?i)\s*[\(\[][^\)\]]*\b(remaster(ed)?|deluxe|edition|anniversary|expanded|bonus|promo|edit|remix|rmx|mono|stereo|explicit|clean)\b[^\)\]]*[\)\]]`)
	junkDash  = regexp.MustCompile(`(?i)\s+-\s+[^-]*\b(remaster(ed)?|promo|edit|official|remix|rmx|deluxe|edition|mono|stereo|single)\b.*$`)
	junkTail  = regexp.MustCompile(`(?i)\s+(rmx|-)\s*$`)
	hitsRe    = regexp.MustCompile(`(?i)\b(greatest hits|best of|the essential|essentials|collection|anthology)\b`)
)

// cleanTitle drops edition junk: "(Remastered 2003)", " - Promo 7\" Edit",
// " - Official -", "RMX - Remix". A title that would end up empty stays.
func cleanTitle(s string) string {
	t := s
	for {
		u := junkTail.ReplaceAllString(junkDash.ReplaceAllString(junkParen.ReplaceAllString(t, ""), ""), "")
		u = strings.TrimSpace(u)
		if u == t || u == "" {
			break
		}
		t = u
	}
	if t == "" {
		return s
	}
	return t
}

// honestSearch wraps the MusicBrainz search for albums: compilations, live
// albums and hits sets are dropped (unless the wanted title is one), and
// the oldest release group of a name comes first (the original over a
// remaster).
type honestSearch struct{ inner musictrend.Searcher }

func (h honestSearch) Search(ctx context.Context, kind, name, artist string) ([]musictrend.Candidate, error) {
	cs, err := h.inner.Search(ctx, kind, name, artist)
	if err != nil || kind != "album" {
		return cs, err
	}
	wantHits := hitsRe.MatchString(name)
	var out []musictrend.Candidate
	for _, c := range cs {
		if !wantHits && (badSecondary(c.Secondary) || hitsRe.MatchString(c.Name)) {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Date, out[j].Date
		if (a == "") != (b == "") {
			return a != ""
		}
		return a < b
	})
	return out, nil
}

func badSecondary(types []string) bool {
	for _, t := range types {
		switch strings.ToLower(t) {
		case "compilation", "live", "dj-mix", "mixtape/street", "remix":
			return true
		}
	}
	return false
}

// ageDays is the days since a release date ("2026-10-02", "2026-10" or
// "2026"); false when unknown.
func ageDays(date string, now time.Time) (int, bool) {
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if len(date) >= len(layout) {
			if t, err := time.Parse(layout, date[:len(layout)]); err == nil {
				return int(now.UTC().Sub(t).Hours() / 24), true
			}
		}
	}
	return 0, false
}

// recheckAge: a song charting now matched to a release group older than
// two years is checked again.
const recheckAge = 730

// RGInfo reads a release group's first release date from a stored
// MusicBrainz answer (no call).
func RGInfo(stored func(ctx context.Context, kind, mbid string) ([]byte, bool)) func(mbid string) string {
	return func(mbid string) string {
		b, ok := stored(context.Background(), "release-group", mbid)
		if !ok {
			return ""
		}
		var rg struct {
			Date string `json:"first-release-date"`
		}
		if json.Unmarshal(b, &rg) != nil {
			return ""
		}
		return rg.Date
	}
}
