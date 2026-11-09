package subtitles

import (
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
)

// Video holds what a candidate is scored against.
type Video struct {
	Kind        string // movie or episode
	Title       string
	Aliases     []string
	Year        int
	Season      int
	Episode     int
	ReleaseName string // the File's release name, or its file name when it has none
	FPS         float64
}

// Part is one line of a score: what was compared and how many points it gave.
type Part struct {
	Name    string `json:"name"`
	Points  int    `json:"points"`
	Matched bool   `json:"matched"`
}

// Score is a candidate's points out of 100 and the parts behind them. Refused
// says why a candidate can never be picked automatically, such as a wrong
// title, a wrong episode or an unwanted forced track. A refused Total is 0.
type Score struct {
	Total   int    `json:"total"`
	Parts   []Part `json:"parts"`
	Refused string `json:"refused,omitempty"`
}

// weights holds the points for each scored part as {episode, movie}. Each column sums to 100.
var weights = map[string][2]int{
	"name":       {46, 34},
	"year":       {22, 16},
	"season":     {10, 0},
	"episode":    {10, 0},
	"group":      {4, 20},
	"source":     {2, 10},
	"resolution": {1, 5},
	"codec":      {1, 4},
	"audio":      {0, 3},
	"edition":    {0, 2},
	"as_wished":  {2, 3},
	"trusted":    {1, 2},
	"sync":       {1, 1},
}

var partOrder = []string{"name", "year", "season", "episode", "group", "source", "resolution", "codec", "audio", "edition", "as_wished", "trusted", "sync"}

func weight(kind, part string) int {
	if kind == "episode" {
		return weights[part][0]
	}
	return weights[part][1]
}

// Limits on how much work one site answer may cause while scoring.
const (
	MaxReleaseNames      = 50  // release names scored for each candidate
	MaxCandidatesPerSite = 500 // candidates scored for each site in one search
)

// ScoreCandidate scores one candidate for one wanted language. It reads only
// the first MaxReleaseNames release names so a large answer stays cheap.
func ScoreCandidate(v Video, w Want, c sites.Candidate) Score {
	matched := map[string]bool{}
	refuse := func(why string) Score {
		return Score{Refused: why, Parts: []Part{}}
	}
	if c.Forced != w.Forced {
		return refuse("forced does not match the wish")
	}
	show := v.Kind == "episode"
	vr := releasename.Parse(v.ReleaseName, releasename.Hints{Show: show})

	// Keep the candidate's release name that matches the most release parts.
	var best releasename.Result
	bestN, exact := -1, false
	for _, name := range c.ReleaseNames[:min(len(c.ReleaseNames), MaxReleaseNames)] {
		if len(name) > 512 {
			continue
		}
		if foldName(name) == foldName(v.ReleaseName) && foldName(name) != "" {
			exact = true
		}
		r := releasename.Parse(name, releasename.Hints{Show: show})
		n := len(releaseMatches(vr, r))
		if n > bestN {
			best, bestN = r, n
		}
	}

	// Title name.
	name := c.Title
	if name == "" && bestN >= 0 {
		name = best.Title.Value
	}
	switch {
	case c.MatchedByID:
		matched["name"] = true
	case name != "":
		if !sameTitle(name, v.Title, v.Aliases) {
			return refuse("wrong title")
		}
		matched["name"] = true
	}
	// Year.
	year := c.Year
	if year == 0 && bestN >= 0 && best.Year.Conf != releasename.Unknown {
		year = best.Year.Value
	}
	switch {
	case year != 0 && v.Year != 0 && year != v.Year && !c.MatchedByID:
		return refuse("wrong year")
	case c.MatchedByID, year != 0 && year == v.Year:
		matched["year"] = true
	}
	// Season and episode.
	if show {
		se, ep := c.Season, c.Episode
		if se == 0 && bestN >= 0 && best.Show != nil && len(best.Show.Seasons) == 1 {
			se = best.Show.Seasons[0]
		}
		if ep == 0 && bestN >= 0 && best.Show != nil && len(best.Show.Episodes) > 0 {
			ep = best.Show.Episodes[0]
			if slices.Contains(best.Show.Episodes, v.Episode) {
				ep = v.Episode
			}
		}
		if (se != 0 && se != v.Season) || (ep != 0 && ep != v.Episode) {
			return refuse("wrong episode")
		}
		matched["season"], matched["episode"] = se != 0, ep != 0
	}
	// Release parts.
	if exact {
		for _, p := range []string{"group", "source", "resolution", "codec", "audio", "edition"} {
			matched[p] = true
		}
	} else if bestN >= 0 {
		for _, p := range releaseMatches(vr, best) {
			matched[p] = true
		}
	}
	// Hearing impaired as wanted. Forced was already checked above.
	switch w.HearingImpaired {
	case HIOnly:
		matched["as_wished"] = c.HearingImpaired
	case HIEither:
		matched["as_wished"] = true
	default:
		matched["as_wished"] = !c.HearingImpaired
	}
	matched["trusted"] = c.Trusted
	matched["sync"] = c.Corrected || (c.FPS > 0 && v.FPS > 0 && math.Abs(c.FPS-v.FPS) < 0.01)

	s := Score{Parts: []Part{}}
	for _, p := range partOrder {
		pts := weight(v.Kind, p)
		if pts == 0 {
			continue
		}
		part := Part{Name: p, Matched: matched[p]}
		if matched[p] {
			part.Points = pts
			s.Total += pts
		}
		s.Parts = append(s.Parts, part)
	}
	return s
}

// releaseMatches lists the release parts two parsed names have in common.
func releaseMatches(v, c releasename.Result) []string {
	var out []string
	if v.Group.Value != "" && strings.EqualFold(v.Group.Value, c.Group.Value) {
		out = append(out, "group")
	}
	if v.Source.Value != "" && v.Source.Value == c.Source.Value {
		out = append(out, "source")
	}
	if v.Resolution.Value != 0 && v.Resolution.Value == c.Resolution.Value {
		out = append(out, "resolution")
	}
	if v.Codec.Value != "" && v.Codec.Value == c.Codec.Value {
		out = append(out, "codec")
	}
	if v.Audio.Format != "" && v.Audio.Format == c.Audio.Format {
		out = append(out, "audio")
	}
	for _, e := range v.Edition {
		if slices.Contains(c.Edition, e) {
			out = append(out, "edition")
			break
		}
	}
	return out
}

func foldName(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func sameTitle(name, title string, aliases []string) bool {
	n := foldName(name)
	if n == "" {
		return false
	}
	for _, t := range append([]string{title}, aliases...) {
		if foldName(t) == n {
			return true
		}
	}
	return false
}

// Scored is a candidate paired with its score.
type Scored struct {
	sites.Candidate
	Score    Score `json:"score"`
	Priority int   `json:"-"` // site priority, lower goes first
	Order    int   `json:"-"` // position of the site in the list
}

// Rank sorts candidates best first by score, then trust, site priority, most
// downloads and site order. Refused candidates always sort last.
func Rank(cs []Scored) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if (a.Score.Refused == "") != (b.Score.Refused == "") {
			return a.Score.Refused == ""
		}
		if a.Score.Total != b.Score.Total {
			return a.Score.Total > b.Score.Total
		}
		if a.Trusted != b.Trusted {
			return a.Trusted
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if a.Downloads != b.Downloads {
			return a.Downloads > b.Downloads
		}
		return a.Order < b.Order
	})
}
