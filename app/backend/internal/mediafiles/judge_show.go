package mediafiles

import (
	"fmt"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// matchShow picks the show among same-named candidates. Show names carry
// no year as a rule, so a single candidate is sure; a year in the name
// tells shows of the same name apart.
func matchShow(p releasename.Result, cands []*TitleRef) Match {
	if p.Year.Value > 0 {
		for _, t := range cands {
			if t.Year == p.Year.Value {
				return Match{Title: t, Conf: releasename.Sure, Detail: fmt.Sprintf("%s (%d) by clean name and year", t.Name, t.Year)}
			}
		}
	}
	if len(cands) == 1 {
		return Match{Title: cands[0], Conf: releasename.Sure, Detail: cands[0].Name + " by clean name"}
	}
	return Match{Conf: releasename.Unknown, Detail: fmt.Sprintf("%d shows are called %q and the name gives no year that tells them apart", len(cands), p.Title.Value)}
}

// EpisodeRef is one episode of a show as the matcher sees it.
type EpisodeRef struct {
	ItemID   int64
	Season   int
	Episode  int
	Absolute int    // 0: none
	AirDay   string // YYYY-MM-DD, "" when unknown
	// AltSeason and AltEpisode are how release names number the episode
	// when that differs (an anime part TMDB keeps inside one season); 0: none.
	AltSeason  int
	AltEpisode int
}

// Label names an episode the way people write it: S01E02.
func (e EpisodeRef) Label() string { return fmt.Sprintf("S%02dE%02d", e.Season, e.Episode) }

// Named is the season and episode release names use for the episode.
func (e EpisodeRef) Named() (int, int) {
	if e.AltSeason > 0 {
		return e.AltSeason, e.AltEpisode
	}
	return e.Season, e.Episode
}

// isPack says the Release holds whole seasons.
func isPack(p releasename.Result) bool {
	switch p.Kind.Value {
	case releasename.KindSeasonPack, releasename.KindMultiSeason, releasename.KindComplete:
		return true
	}
	return false
}

// IsPack says a show Release holds one or more whole seasons.
func IsPack(p releasename.Result) bool { return isPack(p) }

// MatchEpisodes finds the episodes of a show that a Release holds, using
// the show's episode order (aired, daily or absolute). It returns the
// episodes found and, when some numbers name no episode, why.
func MatchEpisodes(p releasename.Result, order string, eps []EpisodeRef) ([]EpisodeRef, string) {
	sh := p.Show
	if sh == nil {
		return nil, "the name gives no episode numbers"
	}
	if sh.Conf == releasename.Guess {
		return nil, strings.Join(p.Notes, "; ") + "; pick the episode by hand"
	}
	var out []EpisodeRef
	find := func(ok func(EpisodeRef) bool) {
		for _, e := range eps {
			if ok(e) {
				out = append(out, e)
			}
		}
	}
	switch p.Kind.Value {
	case releasename.KindSeasonPack, releasename.KindMultiSeason:
		// An episode sits in the season release names give it; the stored
		// season counts only when that finds nothing.
		find(func(e EpisodeRef) bool { s, _ := e.Named(); return slices.Contains(sh.Seasons, s) })
		if len(out) == 0 {
			find(func(e EpisodeRef) bool { return slices.Contains(sh.Seasons, e.Season) })
		}
		if len(out) == 0 {
			return nil, fmt.Sprintf("the show has no episodes in season %s", seasonList(sh.Seasons))
		}
		return out, ""
	case releasename.KindComplete:
		find(func(e EpisodeRef) bool { return e.Season > 0 })
		if len(out) == 0 {
			return nil, "the show has no episodes yet"
		}
		return out, ""
	case releasename.KindDaily:
		find(func(e EpisodeRef) bool { return e.AirDay == sh.AirDate })
		if len(out) == 0 {
			return nil, "no episode aired on " + sh.AirDate
		}
		return out, ""
	case releasename.KindAbsolute:
		var missing []string
		for _, n := range sh.Absolute {
			i := slices.IndexFunc(eps, func(e EpisodeRef) bool { return e.Absolute == n })
			if i < 0 {
				missing = append(missing, fmt.Sprint(n))
				continue
			}
			out = append(out, eps[i])
		}
		if len(missing) > 0 {
			return nil, "no episode has the running number " + strings.Join(missing, ", ")
		}
		return out, ""
	}
	// Season and episode numbers (episodes, specials, mini-series parts).
	if len(sh.Seasons) == 0 || len(sh.Episodes) == 0 {
		return nil, "the name gives no episode numbers"
	}
	var missing []string
	for _, n := range sh.Episodes {
		// Release-name numbers first, as the old apps read scene numbers;
		// then the stored ones.
		i := slices.IndexFunc(eps, func(e EpisodeRef) bool { s, x := e.Named(); return s == sh.Seasons[0] && x == n })
		if i < 0 {
			i = slices.IndexFunc(eps, func(e EpisodeRef) bool { return e.Season == sh.Seasons[0] && e.Episode == n })
		}
		if i < 0 {
			missing = append(missing, fmt.Sprintf("S%02dE%02d", sh.Seasons[0], n))
			continue
		}
		out = append(out, eps[i])
	}
	if len(missing) > 0 {
		return nil, "the show has no episode " + strings.Join(missing, ", ")
	}
	_ = order // every order keeps season and episode numbers; daily and absolute names are matched above
	return out, ""
}

func seasonList(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, ", ")
}

// episodeOrder ranks two show Releases of equal quality, points and
// protocol: a season pack first; then, for anime (running numbers), more
// episodes; for other shows, fewer; then the lower first episode.
func episodeOrder(a, b releasename.Result) (int, string) {
	if !a.IsShow() || !b.IsShow() {
		return 0, ""
	}
	if pa, pb := isPack(a), isPack(b); pa != pb {
		if pa {
			return -1, "on season pack"
		}
		return 1, "on season pack"
	}
	na, nb := episodeCount(a), episodeCount(b)
	if na != nb {
		more := a.Kind.Value == releasename.KindAbsolute || b.Kind.Value == releasename.KindAbsolute
		if (na > nb) == more {
			return -1, "on episodes held"
		}
		return 1, "on episodes held"
	}
	if fa, fb := firstEpisode(a), firstEpisode(b); fa != fb {
		if fa < fb {
			return -1, "on episode number"
		}
		return 1, "on episode number"
	}
	return 0, ""
}

func episodeCount(p releasename.Result) int {
	if p.Show == nil {
		return 0
	}
	return max(len(p.Show.Episodes), len(p.Show.Absolute), 1)
}

func firstEpisode(p releasename.Result) int {
	switch {
	case p.Show == nil:
		return 0
	case len(p.Show.Episodes) > 0:
		return p.Show.Episodes[0]
	case len(p.Show.Absolute) > 0:
		return p.Show.Absolute[0]
	}
	return 0
}

// Gains says whether a Release would be a better copy than a File in
// place under the rules (pick-rules.md section 4); no File in place is
// always a gain. Used per episode for season packs.
func Gains(rel ReleaseFacts, rules PickRulesBody, cur *Current) bool {
	if cur == nil {
		return true
	}
	idx := stepIndex(rules, StepFor(rel.Parse))
	if idx < 0 || !rules.Steps[idx].Allowed {
		return false
	}
	var v Verdict
	v.better(rel, rules, cur, idx, sumPoints(rel.Parse, rules))
	return !v.failed()
}
