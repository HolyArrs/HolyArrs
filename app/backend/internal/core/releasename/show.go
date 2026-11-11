package releasename

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Show patterns, tried in this order; the first that fits wins.
var (
	sep           = `[\s._]`
	reMultiSeason = regexp.MustCompile(`(?i)\bS(\d{1,2})` + sep + `?-` + sep + `?S(\d{1,2})\b`)
	reEpisodes    = regexp.MustCompile(`(?i)\bS(\d{1,3})` + sep + `?E(\d{1,4})((?:` + sep + `?-?` + sep + `?E\d{1,4}|-\d{1,4})*)\b`)
	reEpMore      = regexp.MustCompile(`(?i)(-)?` + sep + `?E?(\d{1,4})`)
	reCross       = regexp.MustCompile(`(?i)(?:^|[\s._\-\[(])(\d{1,2})x(\d{2,3})\b`)
	reDailyYMD    = regexp.MustCompile(`\b((?:19|20)\d\d)[\s._-](\d\d)[\s._-](\d\d)\b`)
	reDailyDMY    = regexp.MustCompile(`\b(\d\d)[\s._-](\d\d)[\s._-]((?:19|20)\d\d)\b`)
	reComplete    = regexp.MustCompile(`(?i)\bcomplete` + sep + `(?:series|show)\b`)
	reOVA         = regexp.MustCompile(`(?i)\b(?:OVA|OAD|SP)` + sep + `?(\d{1,3})\b`)
	reSeasonDash  = regexp.MustCompile(`(?i)\bS(\d{1,2})\s+-\s+(\d{1,4})\b`)
	// "Season 2 - 05" and "2nd Season - 05". The episode has two digits or
	// more, so "Season 1 - 2" (seasons) and "Season 2 - 1080p" stay out.
	reSeasonWordDash = regexp.MustCompile(`(?i)\b(?:Season` + sep + `?(\d{1,2})|(\d{1,2})(?:st|nd|rd|th)` + sep + `Season)\s+-\s+(\d{2,4})(?:v(\d))?(?:\s?[-~]\s?(\d{2,4}))?\b`)
	reAbsolute       = regexp.MustCompile(`\s-\s(\d{1,4})(?:v(\d))?(?:\s?-\s?(\d{1,4}))?(?:\s|$|[\[(])`)
	reSeason         = regexp.MustCompile(`(?i)\b(?:S|Season` + sep + `?)(\d{1,2})\b`)
	rePart           = regexp.MustCompile(`(?i)\b(?:Part|Pt)` + sep + `?(\d{1,2}|[IVX]{1,5})\b`)
)

var roman = map[string]int{"I": 1, "II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6, "VII": 7, "VIII": 8, "IX": 9, "X": 10,
	"XI": 11, "XII": 12, "XIII": 13, "XIV": 14, "XV": 15}

type showHit struct {
	kind string
	n    ShowNumbers
	fix  int
	note string
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// span lists a to b. A range wider than 500 is no range: nil.
func span(a, b int) []int {
	if b < a {
		return []int{a}
	}
	if b-a > 500 {
		return nil
	}
	out := make([]int, 0, b-a+1)
	for i := a; i <= b; i++ {
		out = append(out, i)
	}
	return out
}

// findShow looks for episode numbers in a name (group and extensions
// already taken off). It returns where the numbers start and end. anime
// says the name began with a [group]; show says it is judged for a show.
func findShow(s string, anime, show bool) (showHit, int, int, bool) {
	if m := reMultiSeason.FindStringSubmatchIndex(s); m != nil {
		a, b := atoi(s[m[2]:m[3]]), atoi(s[m[4]:m[5]])
		if sp := span(a, b); sp != nil {
			return showHit{kind: KindMultiSeason, n: ShowNumbers{Seasons: sp}}, m[0], m[1], true
		}
	}
	if m := reEpisodes.FindStringSubmatchIndex(s); m != nil && episodeRun(s[m[6]:m[7]], atoi(s[m[4]:m[5]])) != nil {
		season, first := atoi(s[m[2]:m[3]]), atoi(s[m[4]:m[5]])
		eps := episodeRun(s[m[6]:m[7]], first)
		h := showHit{kind: KindEpisode, n: ShowNumbers{Seasons: []int{season}, Episodes: eps}}
		switch {
		case season == 0 || first == 0:
			h.kind, h.n.Special = KindSpecial, true
		case len(eps) > 1:
			h.kind = KindMultiEpisode
		}
		return h, m[0], m[1], true
	}
	for _, m := range reCross.FindAllStringSubmatchIndex(s, -1) {
		season, ep := atoi(s[m[2]:m[3]]), atoi(s[m[4]:m[5]])
		if ep == 264 || ep == 265 { // "5.1x264": channels then a codec
			continue
		}
		h := showHit{kind: KindEpisode, n: ShowNumbers{Seasons: []int{season}, Episodes: []int{ep}}}
		if season == 0 || ep == 0 {
			h.kind, h.n.Special = KindSpecial, true
		}
		return h, m[0], m[1], true
	}
	if m := reDailyYMD.FindStringSubmatchIndex(s); m != nil {
		y, mo, d := atoi(s[m[2]:m[3]]), atoi(s[m[4]:m[5]]), atoi(s[m[6]:m[7]])
		if mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
			return showHit{kind: KindDaily, n: ShowNumbers{AirDate: fmt.Sprintf("%04d-%02d-%02d", y, mo, d)}}, m[0], m[1], true
		}
	}
	if m := reDailyDMY.FindStringSubmatchIndex(s); m != nil {
		a, b, y := atoi(s[m[2]:m[3]]), atoi(s[m[4]:m[5]]), atoi(s[m[6]:m[7]])
		d, mo, conf := a, b, Likely // day first unless that cannot be
		if b > 12 && a <= 12 {
			d, mo, conf = b, a, Sure
		} else if a > 12 {
			conf = Sure
		}
		if mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
			h := showHit{kind: KindDaily, n: ShowNumbers{AirDate: fmt.Sprintf("%04d-%02d-%02d", y, mo, d), Conf: conf}}
			return h, m[0], m[1], true
		}
	}
	if m := reComplete.FindStringIndex(s); m != nil {
		return showHit{kind: KindComplete}, m[0], m[1], true
	}
	if m := reOVA.FindStringSubmatchIndex(s); m != nil {
		return showHit{kind: KindSpecial, n: ShowNumbers{Seasons: []int{0}, Episodes: []int{atoi(s[m[2]:m[3]])}, Special: true}}, m[0], m[1], true
	}
	if m := reSeasonDash.FindStringSubmatchIndex(s); m != nil {
		season, ep := atoi(s[m[2]:m[3]]), atoi(s[m[4]:m[5]])
		return showHit{kind: KindEpisode, n: ShowNumbers{Seasons: []int{season}, Episodes: []int{ep}}}, m[0], m[1], true
	}
	if m := reSeasonWordDash.FindStringSubmatchIndex(s); m != nil {
		season := m[2:4]
		if m[2] < 0 {
			season = m[4:6]
		}
		h := showHit{kind: KindEpisode, n: ShowNumbers{Seasons: []int{atoi(s[season[0]:season[1]])}, Episodes: []int{atoi(s[m[6]:m[7]])}}}
		if m[8] >= 0 {
			h.fix = atoi(s[m[8]:m[9]]) - 1
		}
		// "Season 2 - 01-12" is a batch of that season.
		if m[10] >= 0 {
			if sp := span(h.n.Episodes[0], atoi(s[m[10]:m[11]])); len(sp) > 1 {
				h.kind, h.n.Episodes = KindMultiEpisode, sp
			}
		}
		return h, m[0], m[1], true
	}
	// A running number only for an anime name or a show search, never a year.
	if m := reAbsolute.FindStringSubmatchIndex(s); m != nil && (anime || show) && !isYearNum(s[m[2]:m[3]]) {
		a := atoi(s[m[2]:m[3]])
		h := showHit{kind: KindAbsolute, n: ShowNumbers{Absolute: []int{a}}}
		if m[4] >= 0 {
			h.fix = atoi(s[m[4]:m[5]]) - 1
		}
		if m[6] >= 0 {
			h.n.Absolute = span(a, atoi(s[m[6]:m[7]]))
		}
		if h.n.Absolute == nil {
			return showHit{}, 0, 0, false
		}
		if reResWord.MatchString(strconv.Itoa(a)+"p") && len(h.n.Absolute) == 1 {
			h.n.Conf = Guess
			h.note = fmt.Sprintf("%d may be the resolution or the episode number", a)
		}
		end := m[1]
		if end > m[0] && end <= len(s) && strings.ContainsAny(s[end-1:end], "[(") {
			end-- // keep the bracket for the tags
		}
		return h, m[0], end, true
	}
	if m := reSeason.FindStringSubmatchIndex(s); m != nil {
		season := atoi(s[m[2]:m[3]])
		h := showHit{kind: KindSeasonPack, n: ShowNumbers{Seasons: []int{season}}}
		if season == 0 {
			h.n.Special = true
		}
		return h, m[0], m[1], true
	}
	if m := rePart.FindStringSubmatchIndex(s); m != nil {
		v := s[m[2]:m[3]]
		n, err := strconv.Atoi(v)
		if err != nil {
			n = roman[strings.ToUpper(v)]
		}
		// "Part 2 2024" is a movie with a year; a mini-series part has none.
		// Outside a show search, "Part 2" before tags is part of a movie title.
		rest := reSeparators.Split(strings.TrimSpace(s[m[1]:]), -1)
		rest = slices.DeleteFunc(rest, func(t string) bool { return t == "" || t == "-" })
		tagged := slices.ContainsFunc(rest, isTag)
		if n > 0 && (len(rest) == 0 || !reYear.MatchString(rest[0])) && (show || !tagged) {
			return showHit{kind: KindEpisode, n: ShowNumbers{Seasons: []int{1}, Episodes: []int{n}}}, m[0], m[1], true
		}
	}
	return showHit{}, 0, 0, false
}

// parseShow fills a Result for a show Release: the title is what comes
// before the numbers (with a year at its end, if any), the tags what comes
// after. anime says the name began with a [group].
func parseShow(r *Result, h showHit, pre, post string, trailing []string, anime bool, flags map[string]bool, hints Hints) {
	r.Kind = sure(h.kind)
	n := h.n
	if n.Conf == "" {
		n.Conf = Sure
	}
	r.Show = &n
	if h.note != "" {
		r.Notes = append(r.Notes, h.note)
	}
	toks := reSeparators.Split(strings.Trim(strings.TrimSpace(pre), "-"), -1)
	toks = slices.DeleteFunc(toks, func(t string) bool { return t == "" || t == "-" })
	r.Year.Conf = Unknown
	if k := len(toks); k > 1 && reYear.MatchString(toks[k-1]) {
		r.Year = sure(atoi(toks[k-1]))
		toks = toks[:k-1]
	}
	title := strings.Join(toks, " ")
	if m := reArticle.FindStringSubmatch(title); m != nil {
		title = m[2] + " " + m[1]
	}
	r.Title = sure(title)
	// " - Episode Title" after the episode number is how the old apps name
	// Library files. Its words are the episode's title ("Charlotte's Web",
	// "TS-19"), not tags, unless a resolution says the name ends in quality.
	if strings.HasPrefix(post, " - ") && !slices.ContainsFunc(splitTagPairs(reSeparators.Split(post, -1)), func(t string) bool { return reResWord.MatchString(strings.ToLower(t)) }) {
		post = ""
	}
	tags := splitTagPairs(reSeparators.Split(strings.TrimSpace(post), -1))
	tags = slices.DeleteFunc(tags, func(t string) bool { return t == "" || t == "-" })
	tags = append(tags, trailing...)
	readTags(r, tags, flags, hints)
	r.Fix = max(r.Fix, h.fix)
	if anime && r.Source.Conf == Unknown {
		// Fan releases with a [group] in front are web copies unless they say otherwise.
		r.Source = likely("web_dl")
	}
}

// episodeRun reads the episodes after the first one (E03, -E04, -08). It
// is nil when a range is wider than 500.
func episodeRun(more string, first int) []int {
	eps := []int{first}
	for _, x := range reEpMore.FindAllStringSubmatch(more, -1) {
		n := atoi(x[2])
		if x[1] == "-" && n > eps[len(eps)-1] {
			sp := span(eps[len(eps)-1]+1, n)
			if sp == nil {
				return nil
			}
			eps = append(eps, sp...)
		} else if !slices.Contains(eps, n) {
			eps = append(eps, n)
		}
	}
	return eps
}

// isYearNum says a number reads as a year (1900 to 2099).
func isYearNum(v string) bool { return reYear.MatchString(v) }
