package releasename

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Confidence levels (pick-rules.md section 2.3).
const (
	Sure    Conf = "sure"
	Likely  Conf = "likely"
	Guess   Conf = "guess"
	Unknown Conf = "unknown"
)

// Kinds a parse can have besides "movie".
const (
	KindEpisode      = "episode"       // one episode: S01E02, 1x02, Part 2
	KindMultiEpisode = "multi_episode" // S01E02E03, S01E02-E04
	KindSeasonPack   = "season_pack"   // a whole season: S01, Season 1
	KindMultiSeason  = "multi_season"  // S01-S03
	KindComplete     = "complete"      // the whole show, no season numbers
	KindDaily        = "daily"         // by air date: 2026.03.14
	KindAbsolute     = "absolute"      // anime running number: - 112
	KindSpecial      = "special"       // season 0, episode 0, OVA 2
)

// All patterns are compiled once, at start.
var (
	reSitePrefix = regexp.MustCompile(`(?i)^\[?(?:www\.)[^\s\]]+\]?\s*-\s*`)
	reSeparators = regexp.MustCompile(`[\s._()\[\]]+`)
	reLastBrack  = regexp.MustCompile(`\[([^\[\]]+)\]\s*$`)
	reCRC        = regexp.MustCompile(`\s*\[[0-9A-Fa-f]{8}\]\s*$`)
	reLeadBrack  = regexp.MustCompile(`^\[([^\[\]]+)\]\s*`) // a group name may hold spaces: [Lab Fansub]
	reResWord    = regexp.MustCompile(`^(480|576|720|1080|2160)[pi]$`)
	reYear       = regexp.MustCompile(`^(19|20)\d\d$`)
	reParenYear  = regexp.MustCompile(`\s\((19|20)\d\d\)(\s|$)`)
	reChannels   = regexp.MustCompile(`(?:^|[^0-9])([1-9]) ([01])(?:$|[^0-9])`)
	reH26x       = regexp.MustCompile(`\b[hx] ?26([45])\b`)
	reFix        = regexp.MustCompile(`^(proper|repack|rerip)(\d)?$`)
	reArticle    = regexp.MustCompile(`^(.*), (The|A|An)$`)
	reDDP        = regexp.MustCompile(`^(ddp|dd\+|eac3|ac3|dd|dts|aac|truehd|flac|opus|mp3)(\d)?$`)
)

var videoExt = map[string]bool{"mkv": true, "mp4": true, "avi": true, "m4v": true, "wmv": true, "mov": true}

var exeExt = setOf("exe", "scr", "bat", "cmd", "msi", "msp", "lnk", "js", "jse", "vbs", "vbe", "ps1", "psm1", "com", "pif",
	"hta", "jar", "wsf", "wsh", "cpl", "msc", "reg", "scf", "inf", "application", "gadget", "appref-ms", "dll", "sys")

// Tag words: a token that is one of these (or matches a tag pattern) ends the title.
var tagWords = setOf(
	"bluray", "blu-ray", "bdrip", "brrip", "bd", "remux", "uhd", "4k",
	"web", "web-dl", "webdl", "webrip", "web-rip", "hdtv", "pdtv", "dvdrip", "dvd", "hdcam", "cam", "camrip",
	"hdts", "ts", "telesync", "complete", "iso", "bdmv",
	"x264", "x265", "h264", "h265", "hevc", "avc", "av1", "xvid", "vc1", "vc-1", "mpeg2", "10bit", "8bit",
	"dv", "dovi", "hdr", "hdr10", "hdr10plus", "hdr10+", "hlg",
	"atmos", "truehd", "dts-hd", "ma", "aac", "ac3", "eac3", "ddp", "opus", "flac",
	"amzn", "nf", "dsnp", "hmax", "atvp",
	"directors", "extended", "theatrical", "remastered", "imax", "unrated", "anniversary",
	"3d", "sbs", "hsbs", "ou", "hou",
	"proper", "repack", "rerip", "real", "internal", "sample", "featurette", "featurettes", "extras", "trailer",
	"upscaled", "upscale", "dubbed", "hc", "hardsub", "hardcoded", "korsub", "passworded", "password",
	"multi", "dl", "dual",
)

var langWords = map[string]string{
	"english": "en", "eng": "en", "french": "fr", "truefrench": "fr", "vff": "fr", "vfq": "fr",
	"german": "de", "ger": "de", "italian": "it", "ita": "it", "spanish": "es", "spa": "es",
	"russian": "ru", "rus": "ru", "japanese": "ja", "jpn": "ja", "korean": "ko", "kor": "ko",
}

var editions = []struct{ words, name string }{
	{"directors cut", "Directors Cut"}, {"extended", "Extended"}, {"theatrical", "Theatrical"},
	{"remastered", "Remastered"}, {"imax", "IMAX"}, {"unrated", "Unrated"}, {"anniversary", "Anniversary"},
}

var nextYear = time.Now().Year() + 1

// fold maps accented Latin letters to plain ones (standard library only, so
// no Unicode normalisation tables).
var fold = func() map[rune]string {
	m := map[rune]string{}
	add := func(to string, from string) {
		for _, c := range from {
			m[c] = to
		}
	}
	add("a", "àáâãäåāăą")
	add("c", "çćĉċč")
	add("d", "ďđ")
	add("e", "èéêëēĕėęě")
	add("g", "ĝğġģ")
	add("h", "ĥħ")
	add("i", "ìíîïĩīĭįı")
	add("j", "ĵ")
	add("k", "ķ")
	add("l", "ĺļľŀł")
	add("n", "ñńņňŉ")
	add("o", "òóôõöøōŏő")
	add("r", "ŕŗř")
	add("s", "śŝşš")
	add("t", "ţťŧ")
	add("u", "ùúûüũūŭůűų")
	add("w", "ŵ")
	add("y", "ýÿŷ")
	add("z", "źżž")
	m['ß'] = "ss"
	m['æ'] = "ae"
	m['œ'] = "oe"
	return m
}()

// Conf is how sure the parser is about one field.
type Conf string

// Field is one parsed value with its confidence.
type Field[T any] struct {
	Value T    `json:"value"`
	Conf  Conf `json:"conf"`
}

// Audio is the parsed audio track.
type Audio struct {
	Format   string `json:"format,omitempty"`   // aac, ac3, eac3, dts, dts_hd_ma, truehd, flac, mp3, opus
	Channels string `json:"channels,omitempty"` // "5.1", "7.1"
	Atmos    bool   `json:"atmos,omitempty"`
}

// Hints are what the Source sent besides the name.
type Hints struct {
	Size     int64
	FileList []string
	// Show says the name is judged for a show search: a bare "- 12" is then
	// a running number and "Part 2" before tags a mini-series part.
	Show bool
	// Music says the name is judged for an album search: it is read as
	// "Artist - Album (Year) [tags]" or a scene music name.
	Music bool
}

// Result is a parsed Release name.
type Result struct {
	Original   string          `json:"original"`
	Kind       Field[string]   `json:"kind"`
	Title      Field[string]   `json:"title"`
	Year       Field[int]      `json:"year"`
	Resolution Field[int]      `json:"resolution"`
	Source     Field[string]   `json:"source"`
	Codec      Field[string]   `json:"codec"`
	HDR        Field[[]string] `json:"hdr"`
	Audio      Audio           `json:"audio"`
	Languages  []string        `json:"languages,omitempty"`
	Edition    []string        `json:"edition,omitempty"`
	Group      Field[string]   `json:"group"`
	Fix        int             `json:"fix,omitempty"`
	Flags      []string        `json:"flags,omitempty"` // sorted
	Notes      []string        `json:"notes,omitempty"`
	// Show holds the episode numbers of a show Release; nil for a movie.
	Show *ShowNumbers `json:"show,omitempty"`
	// Music holds what an album Release name says; nil for movies and shows.
	Music *MusicInfo `json:"music,omitempty"`
	// Size is the whole Release's size in bytes when the judge knows it
	// (0: not known). Parse leaves it unset.
	Size int64 `json:"-"`
}

// ShowNumbers are the numbers a show Release names.
type ShowNumbers struct {
	Seasons  []int  `json:"seasons,omitempty"`  // one season; several for a multi-season pack
	Episodes []int  `json:"episodes,omitempty"` // empty for a whole season
	AirDate  string `json:"air_date,omitempty"` // YYYY-MM-DD (daily shows)
	Absolute []int  `json:"absolute,omitempty"` // anime running numbers
	Special  bool   `json:"special,omitempty"`  // season 0, episode 0, OVA
	Conf     Conf   `json:"conf"`               // guess when a number may be something else
}

// IsShow says whether the name is a show Release (any kind but movie).
func (r *Result) IsShow() bool { return r.Show != nil }

// Has reports whether the Result carries flag f.
func (r *Result) Has(f string) bool { return slices.Contains(r.Flags, f) }

// Parse reads a Release name.
func Parse(name string, h Hints) Result {
	if h.Music {
		return parseMusic(name, h)
	}
	r := Result{Original: name, Kind: sure("movie")}
	flags := map[string]bool{}
	s := strings.TrimSpace(name)

	if loc := reSitePrefix.FindStringIndex(s); loc != nil {
		s = s[loc[1]:]
		flags["site_prefix"] = true
	}

	// Known extensions at the end. A program extension after a video one hides a program.
	sawExe := false
	for {
		i := strings.LastIndexByte(s, '.')
		if i < 0 {
			break
		}
		ext := strings.ToLower(s[i+1:])
		switch {
		case exeExt[ext]:
			sawExe = true
		case videoExt[ext]:
			if sawExe {
				flags["double_extension"] = true
			}
		default:
			i = -1
		}
		if i < 0 {
			break
		}
		s = s[:i]
	}
	if sawExe {
		flags["executable"] = true
	}
	for _, f := range h.FileList {
		lf := strings.ToLower(f)
		if j := strings.LastIndexByte(lf, '.'); j >= 0 && exeExt[lf[j+1:]] {
			flags["executable"] = true
		}
	}

	if loc := reCRC.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
		flags["crc"] = true
	}

	// Group: the text after the last hyphen, unless that hyphen joins a tag
	// (WEB-DL, DTS-HD) or the text after it is not a group.
	var trailing []string
	if i := strings.LastIndexByte(s, '-'); i > 0 && i < len(s)-1 && s[i-1] != ' ' && s[i+1] != ' ' {
		after := reSeparators.Split(strings.Trim(s[i+1:], " ._"), -1)
		before := s[:i]
		if j := strings.LastIndexAny(before, " ._()[]"); j >= 0 {
			before = before[j+1:]
		}
		compound := tagWords[strings.ToLower(before+"-"+after[0])]
		allTags := true
		for _, t := range after[1:] {
			allTags = allTags && isTag(t)
		}
		// "WEBDL-1080p", the old apps' quality word at the end of a Library
		// file name: the part after the hyphen is a tag, not a group.
		if !compound && allTags && after[0] != "" && !isTag(after[0]) {
			r.Group = sure(after[0])
			trailing = after[1:]
			s = s[:i]
		}
	}
	if r.Group.Value == "" {
		if m := reLastBrack.FindStringSubmatchIndex(s); m != nil {
			inner := s[m[2]:m[3]]
			if !strings.ContainsAny(inner, " .") && !isTag(inner) {
				r.Group = sure(inner)
				s = s[:m[0]]
			}
		}
	}
	if r.Group.Value == "" {
		r.Group.Conf = Unknown
	}

	// A leading [group] (anime) and the episode numbers make a show Release.
	body, lead := s, ""
	if m := reLeadBrack.FindStringSubmatchIndex(s); m != nil && !slices.ContainsFunc(strings.Fields(s[m[2]:m[3]]), isTag) {
		body, lead = s[m[1]:], s[m[2]:m[3]]
	}
	// Underscores are word characters to the patterns; read them as spaces
	// (same length, so the places found stay right).
	body = strings.ReplaceAll(body, "_", " ")
	if sh, start, end, ok := findShow(body, lead != "", h.Show); ok {
		if lead != "" && r.Group.Conf == Unknown {
			r.Group = sure(lead)
		}
		parseShow(&r, sh, body[:start], body[end:], trailing, lead != "", flags, h)
		for f := range flags {
			r.Flags = append(r.Flags, f)
		}
		slices.Sort(r.Flags)
		return r
	}

	toks := reSeparators.Split(strings.TrimSpace(s), -1)
	toks = slices.DeleteFunc(toks, func(t string) bool { return t == "" })
	toks = append(splitTagPairs(toks), trailing...)

	// Anchor: the first tag, and the last year before it. A year in
	// brackets, as the old apps name Library folders and files
	// ("Charlotte's Web (1973) Bluray-1080p"), ends the title, so title
	// words before it are never read as tags.
	titleUpTo := 0
	if m := reParenYear.FindStringIndex(s); m != nil {
		pre := reSeparators.Split(strings.TrimSpace(s[:m[0]]), -1)
		titleUpTo = len(slices.DeleteFunc(pre, func(t string) bool { return t == "" }))
	}
	first := len(toks)
	for i, t := range toks {
		if i > titleUpTo && isTag(t) {
			first = i
			break
		}
	}
	yearAt := -1
	for i := 1; i < first; i++ {
		if reYear.MatchString(toks[i]) && i+1 < len(toks) {
			if y, _ := strconv.Atoi(toks[i]); y <= nextYear {
				yearAt = i
			}
		}
	}
	end := first
	if yearAt >= 0 {
		end = yearAt
		y, _ := strconv.Atoi(toks[yearAt])
		r.Year = sure(y)
	} else {
		r.Year.Conf = Unknown
	}
	title := strings.Join(toks[:end], " ")
	if m := reArticle.FindStringSubmatch(title); m != nil {
		title = m[2] + " " + m[1]
	}
	r.Title = sure(title)
	if yearAt < 0 && end == 1 && reNumber(toks[0]) {
		// "1871.1080p": the number is the title, the year is open.
		r.Year.Conf = Guess
		r.Title.Conf = Likely
	}

	tags := toks[end:]
	if yearAt >= 0 {
		tags = slices.Delete(slices.Clone(tags), 0, 1)
	}
	readTags(&r, tags, flags, h)

	for f := range flags {
		r.Flags = append(r.Flags, f)
	}
	slices.Sort(r.Flags)
	return r
}

// CleanName is the form names are matched in: lower case, accents removed,
// punctuation removed, "&" read as "and", and a leading "the", "a" or "an" dropped.
func CleanName(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if f, ok := fold[c]; ok {
			b.WriteString(f)
			continue
		}
		switch {
		case c == '&':
			b.WriteString(" and ")
		case unicode.IsLetter(c) || unicode.IsDigit(c):
			b.WriteRune(c)
		case unicode.Is(unicode.Mn, c):
			// combining mark: drop
		default:
			b.WriteByte(' ')
		}
	}
	words := strings.Fields(b.String())
	if len(words) > 1 && (words[0] == "the" || words[0] == "a" || words[0] == "an") {
		words = words[1:]
	}
	return strings.Join(words, " ")
}

func sure[T any](v T) Field[T] { return Field[T]{v, Sure} }

func likely[T any](v T) Field[T] { return Field[T]{v, Likely} }

func setOf(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// splitTagPairs splits a word like "WEBDL-1080p" or "Bluray-720p" (the
// old apps' quality word) into its two tags; other words stay as they are.
func splitTagPairs(toks []string) []string {
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		if a, b, ok := strings.Cut(t, "-"); ok && a != "" && b != "" && !tagWords[strings.ToLower(t)] && (isTag(a) || isOldAppSource(a)) && isTag(b) {
			out = append(out, a, b)
			continue
		}
		out = append(out, t)
	}
	return out
}

// isOldAppSource is a source word only the old apps' quality names use.
func isOldAppSource(tok string) bool {
	switch strings.ToLower(tok) {
	case "webdl", "bluray", "remux", "hdtv", "sdtv", "webrip":
		return true
	}
	return false
}

func isTag(tok string) bool {
	t := strings.ToLower(tok)
	if tagWords[t] || reResWord.MatchString(t) || reDDP.MatchString(t) || reFix.MatchString(t) {
		return true
	}
	if _, ok := langWords[t]; ok {
		return true
	}
	return false
}

func reNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

func readTags(r *Result, tags []string, flags map[string]bool, h Hints) {
	low := make([]string, len(tags))
	for i, t := range tags {
		low[i] = strings.ToLower(t)
	}
	line := " " + strings.Join(low, " ") + " "
	has := func(words ...string) bool {
		for _, w := range words {
			if slices.Contains(low, w) {
				return true
			}
		}
		return false
	}

	// Resolution. Two different values make it a guess; the later one wins.
	var seen []int
	for _, t := range low {
		if m := reResWord.FindStringSubmatch(t); m != nil {
			v, _ := strconv.Atoi(m[1])
			seen = append(seen, v)
		} else if t == "4k" || t == "uhd" {
			seen = append(seen, 2160)
		}
	}
	if len(seen) > 0 {
		r.Resolution = sure(seen[len(seen)-1])
		for _, v := range seen {
			if v != r.Resolution.Value {
				r.Resolution.Conf = Guess
				r.Notes = append(r.Notes, fmt.Sprintf("read as %dp; the name also says %dp", r.Resolution.Value, v))
				break
			}
		}
	} else {
		r.Resolution.Conf = Unknown
	}

	// Source.
	switch {
	case has("iso", "bdmv", "video_ts") || (has("complete") && has("bluray", "blu-ray")):
		r.Source = sure("disc_image")
		flags["disc_image"] = true
	case has("remux"):
		r.Source = sure("remux")
	case has("bluray", "blu-ray", "bdrip", "brrip", "bd"):
		r.Source = sure("bluray")
	case has("web-dl", "webdl"):
		r.Source = sure("web_dl")
	case has("webrip", "web-rip"):
		r.Source = sure("web_rip")
	case has("web"):
		r.Source = likely("web_dl")
	case has("hdtv", "pdtv"):
		r.Source = sure("tv")
	case has("dvdrip", "dvd"):
		r.Source = sure("dvd")
	case has("hdcam", "cam", "camrip"):
		r.Source = sure("cam")
	case has("hdts", "ts", "telesync"):
		r.Source = sure("telesync")
	default:
		r.Source.Conf = Unknown
	}
	if r.Resolution.Conf == Unknown {
		switch r.Source.Value {
		case "bluray", "remux", "disc_image":
			r.Resolution = likely(1080)
		case "dvd":
			r.Resolution = likely(480)
		}
	}

	// Codec.
	switch {
	case reH26x.MatchString(line):
		if reH26x.FindStringSubmatch(line)[1] == "5" {
			r.Codec = sure("h265")
		} else {
			r.Codec = sure("h264")
		}
	case has("hevc"):
		r.Codec = sure("h265")
	case has("avc"):
		r.Codec = sure("h264")
	case has("av1"):
		r.Codec = sure("av1")
	case has("xvid"):
		r.Codec = sure("xvid")
	case has("vc1", "vc-1"):
		r.Codec = sure("vc1")
	case has("mpeg2"):
		r.Codec = sure("mpeg2")
	default:
		r.Codec.Conf = Unknown
	}

	// HDR.
	var hdr []string
	if has("dv", "dovi") || strings.Contains(line, " dolby vision ") {
		hdr = append(hdr, "dv")
	}
	switch {
	case has("hdr10plus", "hdr10+"):
		hdr = append(hdr, "hdr10plus")
	case has("hdr10", "hdr"):
		hdr = append(hdr, "hdr10")
	}
	if has("hlg") {
		hdr = append(hdr, "hlg")
	}
	if hdr != nil {
		r.HDR = sure(hdr)
	} else {
		r.HDR.Conf = Likely // no HDR words: SDR
	}

	// Audio.
	for _, t := range low {
		m := reDDP.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		f := map[string]string{"ddp": "eac3", "dd+": "eac3", "eac3": "eac3", "ac3": "ac3", "dd": "ac3",
			"dts": "dts", "aac": "aac", "truehd": "truehd", "flac": "flac", "opus": "opus", "mp3": "mp3"}[m[1]]
		if r.Audio.Format == "" || f == "truehd" {
			r.Audio.Format = f
		}
	}
	if has("dts-hd") {
		r.Audio.Format = "dts_hd_ma"
	}
	if m := reChannels.FindStringSubmatch(line); m != nil {
		r.Audio.Channels = m[1] + "." + m[2]
	}
	r.Audio.Atmos = has("atmos")

	// Languages.
	for _, t := range low {
		if code, ok := langWords[t]; ok && !slices.Contains(r.Languages, code) {
			r.Languages = append(r.Languages, code)
		}
	}
	if has("multi") {
		flags["multi"] = true
	}
	if has("dl", "dual") {
		flags["dual"] = true
	}

	// Edition.
	for _, e := range editions {
		if strings.Contains(line, " "+e.words+" ") {
			r.Edition = append(r.Edition, e.name)
		}
	}

	// Fix number: PROPER or REPACK = 1, PROPER2 = 2.
	for _, t := range low {
		if m := reFix.FindStringSubmatch(t); m != nil {
			n := 1
			if m[2] != "" {
				n, _ = strconv.Atoi(m[2])
			}
			r.Fix = max(r.Fix, n)
		}
	}

	// Flags.
	set := func(f string, words ...string) {
		if has(words...) {
			flags[f] = true
		}
	}
	set("three_d", "3d", "sbs", "hsbs", "ou", "hou")
	set("hardcoded_subs", "hc", "hardsub", "hardcoded", "korsub")
	set("sample", "sample")
	set("password_hint", "passworded", "password")
	set("extras", "featurette", "featurettes", "extras", "trailer")
	set("upscaled", "upscaled", "upscale")
	set("dubbed", "dubbed")
	set("internal", "internal")
	_ = h
}
