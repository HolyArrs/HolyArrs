package releasename

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Show patterns, tried in this order; the first that fits wins. The order
// follows the old apps: the most specific forms first, the loose ones
// (a bare running number, a season alone, a part) last, so a plain S01E05
// is read by the first patterns and movie names never reach the loose ones.
var (
	sep = `[\s._]`
	// A season number: one to three digits, or a year (S2016E05, 2016x05).
	seasonNum = `(\d{1,3}|(?:19|20)\d\d)`
	// Between the season and the episode: nothing, one separator, "-" or " - ".
	seGap         = `(?:` + sep + `?-` + sep + `?|` + sep + `)?`
	reMultiSeason = regexp.MustCompile(`(?i)\bS(\d{1,2})` + sep + `?-` + sep + `?S(\d{1,2})\b`)
	// "S01-03", "S01.S02.S03", "Season 1-3", "Seasons 1 - 3", "Seasons 1 to 3".
	reSeasonRange = regexp.MustCompile(`(?i)\b(?:S(\d{1,2})-(\d{1,2})|S(\d{1,2})(?:[\s.]S\d{1,2})*[\s.]S(\d{1,2})|Season` + sep + `?(\d{1,2})-(\d{1,2})|Seasons` + sep + `?(\d{1,2})` + sep + `?(?:-|to)` + sep + `?(\d{1,2}))\b`)
	// S01E05, S01.E05, S01-E05, S01 - E05, S01.Ep05, ValleyS01E05, S01E05a,
	// then more episodes: E06, -06, -E06, +E06, &E06, .S01E06, " - S01E06".
	reEpisodes = regexp.MustCompile(`(?i)(?:\b|(?-i:[a-z]))S` + seasonNum + seGap + `Ep?\.?(\d{1,4})(?-i:[ab])?((?:(?:` + sep + `?[-+&]?` + sep + `?|\s-\s)(?:S\d{1,4})?Ep?\d{1,4}|-\d{1,4})*)\b`)
	reEpMore   = regexp.MustCompile(`(?i)(-)?` + sep + `?(?:S(\d{1,4}))?(?:Ep?)?(\d{1,4})`)
	// "Season 1 Episode 5".
	reSeasonEpisode = regexp.MustCompile(`(?i)\bSeason` + sep + `?(\d{1,3})` + sep + `?(?:-` + sep + `?)?Episode` + sep + `?(\d{1,4})\b`)
	// 1x05, 1x5, 2016x05, then -06, x06, " - 1x06".
	reCross      = regexp.MustCompile(`(?i)(?:^|[\s._\-\[(])(\d{1,2}|(?:19|20)\d\d)x(\d{1,3})((?:-x?\d{1,3}|x\d{1,3}|(?:\s?-\s?|[._])\d{1,2}x\d{1,3})*)\b`)
	reCrossMore  = regexp.MustCompile(`\d{1,3}$`)
	reDailyYMD   = regexp.MustCompile(`\b((?:19|20)\d\d)[\s._-](\d\d)[\s._-](\d\d)\b`)
	reDailyDMY   = regexp.MustCompile(`\b(\d\d)[\s._-](\d\d)[\s._-]((?:19|20)\d\d)\b`)
	reDailyFlat  = regexp.MustCompile(`\b((?:19|20)\d\d)(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\b`)
	reDailyWord  = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th)?` + sep + `(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*` + sep + `((?:19|20)\d\d)\b`)
	reComplete   = regexp.MustCompile(`(?i)\bcomplete` + sep + `(?:series|show)\b`)
	reOVA        = regexp.MustCompile(`(?i)\b(?:OVA|OAD|SP)` + sep + `?(\d{1,3})\b`)
	reSeasonDash = regexp.MustCompile(`(?i)\bS(\d{1,2})\s+-\s+(\d{1,4})\b`)
	// "Season 2 - 05" and "2nd Season - 05". The episode has two digits or
	// more, so "Season 1 - 2" (seasons) and "Season 2 - 1080p" stay out.
	reSeasonWordDash = regexp.MustCompile(`(?i)\b(?:Season` + sep + `?(\d{1,2})|(\d{1,2})(?:st|nd|rd|th)` + sep + `Season)\s+-\s+(\d{2,4})(?:v(\d))?(?:\s?[-~]\s?(\d{2,4}))?\b`)
	reAbsolute       = regexp.MustCompile(`\s[-~]\s(\d{1,4})(?:v(\d))?(?:\s?[-~]\s?(\d{1,4}))?(?:\s|$|[\[(])`)
	// Anime forms read only after a leading [group]: "Title 112 [1080p]",
	// "[05]", "[01-12]", "Title E112", "Title Episode 112", "- 01~12".
	reAnimeBare  = regexp.MustCompile(`\s(\d{2,4})(?:v(\d))?\s*(?:[\[(]|$)`)
	reAnimeBrack = regexp.MustCompile(`\[(\d{2,4})(?:-(\d{2,4}))?\]`)
	// "E02", "Ep02", "Episode 2" with no season.
	reBareEp = regexp.MustCompile(`(?i)\b(?:E|Ep|Episode` + sep + `?)(\d{1,4})\b`)
	// "2of6", "2.of.6".
	reOf = regexp.MustCompile(`(?i)\b(\d{1,2})` + sep + `?of` + sep + `?(\d{1,2})\b`)
	// Spanish "[Cap.105]" (season 1, episode 05), Turkish "1.Sezon.05.Bolum".
	reCap   = regexp.MustCompile(`(?i)\bCap[.\s]?(\d{1,2})(\d\d)\b`)
	reSezon = regexp.MustCompile(`(?i)\b(\d{1,2})` + sep + `Sezon` + sep + `(\d{1,4})` + sep + `B[oö]l[uü]m\b`)
	// A season alone; a year is a season only after an S (S2016), never
	// after a word ("The Season 2019" is a movie).
	reSeason = regexp.MustCompile(`(?i)\b(?:S(\d{1,2}|(?:19|20)\d\d)|(?:Season|Stagione|Saison|Temporada|Staffel)` + sep + `?(\d{1,2}))\b`)
	// "Сезон: 1" (Russian trackers);  does not see Cyrillic letters.
	reSeasonCyr = regexp.MustCompile(`(?i)(?:^|[\s._/(\[-])Сезон[\s._:]*(\d{1,2})(?:\D|$)`)
	rePart      = regexp.MustCompile(`(?i)\b(?:Part|Pt)` + sep + `?(\d{1,2}|[IVX]{1,5}|one|two|three|four|five|six|seven|eight|nine|ten)\b`)
)

var roman = map[string]int{"ONE": 1, "TWO": 2, "THREE": 3, "FOUR": 4, "FIVE": 5, "SIX": 6, "SEVEN": 7, "EIGHT": 8, "NINE": 9, "TEN": 10, "I": 1, "II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6, "VII": 7, "VIII": 8, "IX": 9, "X": 10,
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
	if m := reSeasonRange.FindStringSubmatchIndex(s); m != nil {
		for g := 2; g < 18; g += 4 {
			if m[g] >= 0 {
				if sp := span(atoi(s[m[g]:m[g+1]]), atoi(s[m[g+2]:m[g+3]])); len(sp) > 1 {
					return showHit{kind: KindMultiSeason, n: ShowNumbers{Seasons: sp}}, m[0], m[1], true
				}
			}
		}
	}
	if m := reEpisodes.FindStringSubmatchIndex(s); m != nil {
		season, first := atoi(s[m[2]:m[3]]), atoi(s[m[4]:m[5]])
		if eps := episodeRun(s[m[6]:m[7]], season, first); eps != nil {
			return episodeHit(season, eps), m[2] - 1, m[1], true // m[2]-1: the S, not a title letter before it
		}
	}
	if m := reSeasonEpisode.FindStringSubmatchIndex(s); m != nil {
		return episodeHit(atoi(s[m[2]:m[3]]), []int{atoi(s[m[4]:m[5]])}), m[0], m[1], true
	}
	for _, m := range reCross.FindAllStringSubmatchIndex(s, -1) {
		season, ep := atoi(s[m[2]:m[3]]), atoi(s[m[4]:m[5]])
		if ep == 264 || ep == 265 { // "5.1x264": channels then a codec
			continue
		}
		h := showHit{kind: KindEpisode, n: ShowNumbers{Seasons: []int{season}, Episodes: []int{ep}}}
		// "1x01-02" / "1x01x02": the file holds every episode up to the last.
		if last := reCrossMore.FindString(s[m[6]:m[7]]); last != "" {
			if sp := span(ep, atoi(last)); len(sp) > 1 && len(sp) <= 100 {
				h.kind, h.n.Episodes = KindMultiEpisode, sp
			}
		}
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
	if m := reDailyFlat.FindStringSubmatchIndex(s); m != nil {
		return showHit{kind: KindDaily, n: ShowNumbers{AirDate: s[m[2]:m[3]] + "-" + s[m[4]:m[5]] + "-" + s[m[6]:m[7]]}}, m[0], m[1], true
	}
	if m := reDailyWord.FindStringSubmatchIndex(s); m != nil {
		d, mo := atoi(s[m[2]:m[3]]), months[strings.ToLower(s[m[4]:m[5]])]
		if d >= 1 && d <= 31 {
			return showHit{kind: KindDaily, n: ShowNumbers{AirDate: fmt.Sprintf("%s-%02d-%02d", s[m[6]:m[7]], mo, d)}}, m[0], m[1], true
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
	if anime {
		if m := reAnimeBrack.FindStringSubmatchIndex(s); m != nil && !isYearNum(s[m[2]:m[3]]) && !tagNumbers[atoi(s[m[2]:m[3]])] {
			a, b := atoi(s[m[2]:m[3]]), atoi(s[m[2]:m[3]])
			if m[4] >= 0 {
				b = atoi(s[m[4]:m[5]])
			}
			if sp := span(a, b); sp != nil {
				return showHit{kind: KindAbsolute, n: ShowNumbers{Absolute: sp}}, m[0], m[1], true
			}
		}
		if m := reBareEp.FindStringSubmatchIndex(s); m != nil {
			return showHit{kind: KindAbsolute, n: ShowNumbers{Absolute: []int{atoi(s[m[2]:m[3]])}}}, m[0], m[1], true
		}
		if m := reAnimeBare.FindStringSubmatchIndex(s); m != nil && !isYearNum(s[m[2]:m[3]]) && !tagNumbers[atoi(s[m[2]:m[3]])] {
			h := showHit{kind: KindAbsolute, n: ShowNumbers{Absolute: []int{atoi(s[m[2]:m[3]])}}}
			if m[4] >= 0 {
				h.fix = atoi(s[m[4]:m[5]]) - 1
			}
			end := m[1]
			if end > m[0] && strings.ContainsAny(s[end-1:end], "[(") {
				end--
			}
			return h, m[0], end, true
		}
	}
	if m := reCap.FindStringSubmatchIndex(s); m != nil {
		return episodeHit(atoi(s[m[2]:m[3]]), []int{atoi(s[m[4]:m[5]])}), m[0], m[1], true
	}
	if m := reSezon.FindStringSubmatchIndex(s); m != nil {
		return episodeHit(atoi(s[m[2]:m[3]]), []int{atoi(s[m[4]:m[5]])}), m[0], m[1], true
	}
	// A mini-series episode with no season, only when a show is expected
	// and a title comes first (a file named just "E05" needs its folder).
	if show {
		if m := reBareEp.FindStringSubmatchIndex(s); m != nil && strings.TrimSpace(s[:m[0]]) != "" {
			return episodeHit(1, []int{atoi(s[m[2]:m[3]])}), m[0], m[1], true
		}
		if m := reOf.FindStringSubmatchIndex(s); m != nil && atoi(s[m[2]:m[3]]) <= atoi(s[m[4]:m[5]]) {
			return episodeHit(1, []int{atoi(s[m[2]:m[3]])}), m[0], m[1], true
		}
	}
	m := reSeason.FindStringSubmatchIndex(s)
	if m == nil {
		if m = reSeasonCyr.FindStringSubmatchIndex(s); m != nil {
			m = []int{m[0], m[3], m[2], m[3], -1, -1} // end at the number, not the letter after it
		}
	}
	if m != nil {
		g := m[2:4]
		if g[0] < 0 {
			g = m[4:6]
		}
		season := atoi(s[g[0]:g[1]])
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

// latinRun drops the CJK alt-titles that fansub names put before, after or
// inside the Latin title ("新吊带袜天使 New Panty Stocking", "Sword Art Online
// アリシゼーション Alicization"). A word with any CJK character goes, and the
// Latin words keep their order. That includes words that mix both ("Re:ゼロ"),
// since the Latin half alone is no safe name; kanaReading covers those. A "&"
// beside a dropped word goes too. A title with no Latin-only word stays whole.
func latinRun(words []string) []string {
	if !slices.ContainsFunc(words, func(w string) bool { return strings.IndexFunc(w, isLatin) >= 0 && !isCJK(w) }) {
		return words
	}
	out := make([]string, 0, len(words))
	for i, w := range words {
		if isCJK(w) {
			continue
		}
		if w == "&" && (i == 0 || isCJK(words[i-1]) || i == len(words)-1 || isCJK(words[i+1])) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// reBracketEpisodes matches the episode bracket of [Group][Title][01-12][tags]:
// a number, maybe a second one after a hyphen, a tilde or spaces (indexers
// turn the hyphen into a space), then anything.
var reBracketEpisodes = regexp.MustCompile(`^(\d{1,4})(?:v(\d))?(?:$|[\s~-])\s*[~-]?\s*(\d{1,4})?`)

// tagNumbers are resolutions and codec numbers, which are never episodes.
var tagNumbers = map[int]bool{480: true, 576: true, 720: true, 1080: true, 2160: true, 264: true, 265: true}

// bracketTagOnly says a bracket holds only tags, like "1080p" or "10bit".
func bracketTagOnly(s string) bool {
	f := strings.Fields(s)
	return len(f) > 0 && !slices.ContainsFunc(f, func(w string) bool { return !isTag(w) })
}

// bracketTitleShow reads the layout some fansub groups use, with the title in
// a bracket after the group and the episode numbers in the next one:
// "[Group][Title][01-12][1080p][BDRip]". body starts after the group. The
// numbers are a running count, so they are absolute. title is the bracket
// without its CJK alt-titles and kana its spelled-out reading (see
// kanaReading). Everything after the title, including text left in the
// episode bracket ("[03 1080P]"), goes to post as tags. Brackets that hold
// only tags between the two are skipped. If the second bracket does not start
// with an episode number (a resolution, a year, a codec) it is no match.
// backwards says the numbers were a range that runs backwards, which no other
// reading may take. A search for something other than a show needs a
// zero-padded number or a range to accept it.
func bracketTitleShow(body string, anime, show bool) (h showHit, title, kana, post string, ok, backwards bool) {
	if !anime {
		return h, "", "", "", false, false
	}
	t := reLeadBrack.FindStringSubmatchIndex(body)
	if t == nil {
		return h, "", "", "", false, false
	}
	rest := body[t[1]:]
	skipped := ""
	var e []int
	for {
		e = reLeadBrack.FindStringSubmatchIndex(rest)
		if e == nil {
			return h, "", "", "", false, false
		}
		if !bracketTagOnly(rest[e[2]:e[3]]) {
			break
		}
		skipped += rest[:e[1]]
		rest = rest[e[1]:]
	}
	inner := rest[e[2]:e[3]]
	m := reBracketEpisodes.FindStringSubmatchIndex(inner)
	if m == nil {
		return h, "", "", "", false, false
	}
	first := atoi(inner[m[2]:m[3]])
	if isYearNum(inner[m[2]:m[3]]) || tagNumbers[first] {
		return h, "", "", "", false, false
	}
	titleText := body[t[2]:t[3]]
	if slices.ContainsFunc(strings.Fields(titleText), isTag) {
		return h, "", "", "", false, false
	}
	h = showHit{kind: KindAbsolute, n: ShowNumbers{Absolute: []int{first}}}
	if m[4] >= 0 {
		h.fix = atoi(inner[m[4]:m[5]]) - 1
	}
	end := m[1]
	if m[6] >= 0 {
		end = m[6] // a second number is a tag unless it makes a range
		tok := inner[m[6]:]
		if i := strings.IndexAny(tok, " \t"); i >= 0 {
			tok = tok[:i]
		}
		digits := inner[m[6]:m[7]]
		tail := strings.TrimPrefix(tok, digits)
		last := atoi(digits)
		tagLike := isTag(tok) || tagNumbers[last] && tail == "" || tail != "" && strings.ContainsAny(tail[:1], ".kKxXpPiI") || strings.HasPrefix(strings.ToLower(tail), "bit")
		if !tagLike {
			if last < first {
				return h, "", "", "", false, true // runs backwards
			}
			end = m[7]
			if last > first {
				if sp := span(first, last); sp != nil {
					h.n.Absolute = sp
				}
			}
		}
	}
	isRange := len(h.n.Absolute) > 1
	zeroPadded := len(inner[m[2]:m[3]]) > 1 && inner[m[2]] == '0'
	if !show && !isRange && !zeroPadded {
		return h, "", "", "", false, false
	}
	post = skipped + " " + inner[end:] + " " + rest[e[1]:]
	return h, latinTitle(titleText), kanaReading(strings.Fields(titleText)), post, true, false
}

// latinTitle is a bracket title without its CJK alt-titles ("新吊带袜天使 New
// PANTY STOCKING" gives "New PANTY STOCKING").
func latinTitle(s string) string {
	return strings.Join(latinRun(strings.Fields(s)), " ")
}

func isLatin(c rune) bool { return unicode.Is(unicode.Latin, c) && unicode.IsLetter(c) }

// isCJK says a word has a Han, kana or Hangul character.
func isCJK(w string) bool {
	return strings.IndexFunc(w, func(c rune) bool {
		return unicode.In(c, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
	}) >= 0
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
	title := strings.Join(latinRun(toks), " ")
	if m := reArticle.FindStringSubmatch(title); m != nil {
		title = m[2] + " " + m[1]
	}
	r.Title = sure(title)
	r.KanaTitle = kanaReading(toks)
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

// episodeRun reads the episodes after the first one (E03, -E04, -08,
// .S01E06). An episode of another season is left out. It is nil when a
// range is wider than 500.
func episodeRun(more string, season, first int) []int {
	eps := []int{first}
	for _, x := range reEpMore.FindAllStringSubmatch(more, -1) {
		if x[2] != "" && atoi(x[2]) != season {
			continue
		}
		n := atoi(x[3])
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

// episodeHit is one season's episodes; season or episode 0 is a special.
func episodeHit(season int, eps []int) showHit {
	h := showHit{kind: KindEpisode, n: ShowNumbers{Seasons: []int{season}, Episodes: eps}}
	switch {
	case season == 0 || eps[0] == 0:
		h.kind, h.n.Special = KindSpecial, true
	case len(eps) > 1:
		h.kind = KindMultiEpisode
	}
	return h
}

var months = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

// isYearNum says a number reads as a year (1900 to 2099).
func isYearNum(v string) bool { return reYear.MatchString(v) }
