package releasename

import (
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
	reLangCount  = regexp.MustCompile(`^\d+[xX]([A-Za-z]+)$`)
	reSeparators = regexp.MustCompile(`[\s._()\[\]]+`)
	reLastBrack  = regexp.MustCompile(`\[([^\[\]]+)\]\s*$`)
	reCRC        = regexp.MustCompile(`\s*\[[0-9A-Fa-f]{8}\]\s*$`)
	reNumNext    = regexp.MustCompile(`(?i)^[EXS]?\d{1,4}(?:E\d{1,4})?$`)
	reLeadBrack  = regexp.MustCompile(`^\[([^\[\]]+)\]\s*`) // a group name may hold spaces: [Lab Fansub]
	reResWord    = regexp.MustCompile(`^(480|576|720|1080|2160)[pi]$`)
	reYear       = regexp.MustCompile(`^(19|20)\d\d$`)
	reParenYear  = regexp.MustCompile(`\s\((19|20)\d\d\)(\s|$)`)
	reBrackYear  = regexp.MustCompile(`\s\[((?:19|20)\d\d),([^\[\]]*)\]`)
	reChannels   = regexp.MustCompile(`(?:^|[^0-9])([1-9]) ([01])(?:$|[^0-9])`)
	reH26x       = regexp.MustCompile(`\b[hx] ?26([45])\b`)
	reFix        = regexp.MustCompile(`^(proper|repack|rerip)(\d)?$`)
	reArticle    = regexp.MustCompile(`^(.*), (The|A|An)$`)
	reDDP        = regexp.MustCompile(`^(ddpa|ddp|dd\+|eac3|ac3|dd|dts|aac|truehd|flac|opus|mp3|lpcm)(\d)?$`)
)

// repostMarks follow the group at the end of a reposted name.
var repostMarks = setOf("xpost", "obfuscated", "scrambled")

// siteTags are sites whose tag ends a name in brackets.
var siteTags = setOf("rarbg", "eztv", "ettv", "tgx", "yts")

var videoExt = map[string]bool{"mkv": true, "mp4": true, "avi": true, "m4v": true, "wmv": true, "mov": true}

var exeExt = setOf("exe", "scr", "bat", "cmd", "msi", "msp", "lnk", "js", "jse", "vbs", "vbe", "ps1", "psm1", "com", "pif",
	"hta", "jar", "wsf", "wsh", "cpl", "msc", "reg", "scf", "inf", "application", "gadget", "appref-ms", "dll", "sys")

// Tag words: a token that is one of these (or matches a tag pattern) ends the title.
var tagWords = setOf(
	"bluray", "blu-ray", "bdrip", "brrip", "bd", "remux", "bdremux", "uhdremux", "uhd", "4k",
	"web", "web-dl", "webdl", "webrip", "web-rip", "webhd", "web-hd", "hdtv", "pdtv", "dvdrip", "dvd", "hdcam", "cam", "camrip",
	"hdts", "ts", "telesync", "complete", "iso", "bdmv",
	"x264", "x265", "h264", "h265", "hevc", "avc", "av1", "xvid", "vc1", "vc-1", "mpeg2", "10bit", "8bit",
	"dv", "dovi", "hdr", "hdr10", "hdr10plus", "hdr10+", "hlg",
	"atmos", "truehd", "dts-hd", "ma", "aac", "ac3", "eac3", "ddp", "opus", "flac",
	"amzn", "nf", "dsnp", "hmax", "atvp",
	"directors", "extended", "theatrical", "remastered", "imax", "unrated", "anniversary",
	"3d", "sbs", "hsbs", "ou", "hou",
	"proper", "repack", "rerip", "real", "internal", "sample", "featurette", "featurettes", "extras", "trailer",
	"upscaled", "upscale", "aius", "uprez", "upsuhd", "upscaleduhd", "regrade", "regraded", "re-graded", "ai-enhanced", "ai-upscaled", "dubbed", "hc", "hardsub", "hardcoded", "korsub", "passworded", "password",
	"multi", "dl", "dual",
	// Source, size, codec and audio words that are never title words.
	"uhdbd", "uhd-bd", "uhdbdrip", "bdlight", "hddvd", "hd-dvd", "hddvdrip", "bdmux", "brdisk", "bd25", "bd50", "bd66", "bd100",
	"webmux", "ituneshd", "amazonhd", "amazonsd", "amazonuhd", "netflixhd", "netflixuhd", "disneyhd", "hbomaxhd", "maxdomehd",
	"hdrip", "sdtv", "ahdtv", "uhdtv", "hdtvrip", "tvrip", "dsrip", "dvbrip", "rawhd", "dvdr", "dvd-r", "dvd5", "dvd9",
	"dvdscr", "dvdscreener", "bdscr", "telecine", "hdtc", "workprint", "vhsrip", "ldrip", "8k", "fhd",
	"vp9", "divx", "mpeg-2", "ma10p", "hi10", "hi10p", "dts-x", "dtsx", "dts-es", "lpcm", "ddpa",
)

var langWords = map[string]string{
	"english": "en", "eng": "en", "french": "fr", "truefrench": "fr", "vff": "fr", "vfq": "fr",
	"vf": "fr", "vf2": "fr", "vfi": "fr", "vof": "fr",
	"german": "de", "ger": "de", "italian": "it", "ita": "it", "spanish": "es", "spa": "es",
	"russian": "ru", "rus": "ru", "japanese": "ja", "jpn": "ja", "korean": "ko", "kor": "ko",
}

// moreLangWords are read as languages after the title, but never end a title,
// because many are ordinary title words ("My Big Fat Greek Wedding"). Short
// words that clash with names (fin, dan, tur, pol, chi, pl, nl) stay out; hun
// is common in Hungarian release names and needs a tag on both sides.
var moreLangWords = map[string]string{
	"polish": "pl", "dutch": "nl", "flemish": "nl", "portuguese": "pt", "swedish": "sv", "swe": "sv",
	"danish": "da", "norwegian": "no", "finnish": "fi", "czech": "cs", "cze": "cs", "hungarian": "hu", "hun": "hu",
	"greek": "el", "hebrew": "he", "heb": "he", "turkish": "tr", "arabic": "ar", "hindi": "hi",
	"chinese": "zh", "mandarin": "zh", "thai": "th", "vietnamese": "vi", "ukrainian": "uk", "ukr": "uk",
	// Scene words and codes for the same languages.
	"fre": "fr", "fra": "fr", "swissgerman": "de", "videomann": "de", "castellano": "es", "espanol": "es",
	"español": "es", "esp": "es", "latino": "es", "brazilian": "pt", "dublado": "pt", "pt-br": "pt", "por": "pt",
	"pldub": "pl", "pl-dub": "pl", "dub-pl": "pl", "lektor": "pl", "lek-pl": "pl", "pl-lek": "pl",
	"hundub": "hu", "hebdub": "he", "cantonese": "zh", "vie": "vi",
	// More languages, by full name and a few unmistakable scene words.
	"bulgarian": "bg", "bgaudio": "bg", "romanian": "ro", "rodubbed": "ro", "catalan": "ca", "latvian": "lv", "lav": "lv",
	"lithuanian": "lt", "slovak": "sk", "icelandic": "is", "persian": "fa", "farsi": "fa", "bengali": "bn",
	"tamil": "ta", "telugu": "te", "malayalam": "ml", "kannada": "kn", "georgian": "ka", "urdu": "ur",
	"estonian": "et", "serbian": "sr", "croatian": "hr", "slovenian": "sl", "indonesian": "id", "malay": "ms",
	"albanian": "sq", "afrikaans": "af", "marathi": "mr", "tagalog": "tl", "filipino": "tl", "macedonian": "mk",
	"bosnian": "bs", "punjabi": "pa", "gujarati": "gu",
}

// upperLangCodes count only when written in upper case ("CZ", not "cz"), so
// a lower-case word in a name is never read as a language.
var upperLangCodes = map[string]string{"LT": "lt", "CZ": "cs", "PL": "pl", "BG": "bg", "SK": "sk", "DE": "de"}

// isLangWord says t (any case) is a language word or code.
func isLangWord(t string) bool {
	if _, ok := upperLangCodes[t]; ok {
		return true
	}
	t = strings.ToLower(t)
	_, a := langWords[t]
	_, b := moreLangWords[t]
	return a || b
}

var editions = []struct{ words, name string }{
	{"directors cut", "Directors Cut"}, {"extended", "Extended"}, {"theatrical", "Theatrical"},
	{"remastered", "Remastered"}, {"imax", "IMAX"}, {"unrated", "Unrated"}, {"anniversary", "Anniversary"},
	{"final cut", "Final Cut"}, {"special edition", "Special Edition"}, {"open matte", "Open Matte"},
	{"criterion", "Criterion"}, {"uncut", "Uncut"},
}

var nextYear = time.Now().Year() + 1

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
	// MultiLanguages are what MULTi means on the Source the name came from
	// (its per-Source list); a name with MULTi then carries them too.
	MultiLanguages []string
	// NoLanguage are the languages a name from that Source carries when it
	// shows none (its per-Source list).
	NoLanguage []string
}

// Result is a parsed Release name.
type Result struct {
	Original string        `json:"original"`
	Kind     Field[string] `json:"kind"`
	Title    Field[string] `json:"title"`
	// KanaTitle is the title read a second way, with the kana of words that
	// mix Latin letters and kana spelled out ("Re:ゼロ" as "Re:zero"). Empty
	// when no word is like that.
	KanaTitle  string          `json:"-"`
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
	// OriginalLanguage is the matched title's original language and
	// AudioLanguages a library file's audio track languages, when the judge
	// knows them; Parse leaves both unset.
	OriginalLanguage string   `json:"-"`
	AudioLanguages   []string `json:"-"`
	// TitleYear is the matched title's year when the judge knows it (0:
	// not known); a year trait test reads it when the name has no year.
	TitleYear int `json:"-"`
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

	// A repost marker at the end is not the group: some posters send a scene
	// name on with "-xpost" after the release group. Drop it so the group
	// before it is read, and cut a CRC that follows the marker.
	// "-Obfuscated" and "-Scrambled" are posting tools' marks of the same kind.
	repost := false
	for {
		i := strings.LastIndexByte(s, '-')
		if i <= 0 || !repostMarks[strings.ToLower(s[i+1:])] {
			break
		}
		s = s[:i]
		repost = true
		if loc := reCRC.FindStringIndex(s); loc != nil {
			s = s[:loc[0]]
			flags["crc"] = true
		}
	}

	// A site's tag in brackets at the end ("[rarbg]", "[eztv.re]", "[TGx]")
	// is not the group; YTS names carry only their own ("[YTS.MX]").
	siteGroup := ""
	if m := reLastBrack.FindStringSubmatchIndex(s); m != nil {
		site, _, _ := strings.Cut(strings.ToLower(s[m[2]:m[3]]), ".")
		if siteTags[site] {
			if site == "yts" {
				siteGroup = "YTS"
			}
			s = strings.TrimRight(s[:m[0]], " ._-")
		}
	}

	// A leading [group] (anime) is the group; brackets at the end then hold
	// tags or a checksum ("[1080p][Multi-Subs]", "[HEVC-x265]").
	leadGroup := ""
	if m := reLeadBrack.FindStringSubmatch(s); m != nil && !slices.ContainsFunc(strings.Fields(m[1]), isTag) {
		leadGroup = m[1]
	}

	// Group: the text after the last hyphen, unless that hyphen joins a tag
	// (WEB-DL, DTS-HD) or the text after it is not a group.
	var trailing []string
	if leadGroup != "" {
		r.Group = sure(leadGroup)
	} else if i := strings.LastIndexByte(s, '-'); i > 0 && i < len(s)-1 && s[i-1] != ' ' && s[i+1] != ' ' {
		after := reSeparators.Split(strings.Trim(s[i+1:], " ._"), -1)
		before := s[:i]
		if j := strings.LastIndexAny(before, " ._()[]"); j >= 0 {
			before = before[j+1:]
		}
		// A name whose marker was cut may have a hyphen inside its title
		// (Spider-Man); the words after it are then not a group.
		compound := tagWords[strings.ToLower(before+"-"+after[0])] || (repost && len(after) > 1)
		// "S01E01-02", "1x01-02", a date: a number after the hyphen
		// that the episode reader takes as part of the episode numbers is
		// not a group. One reader decides, so a new episode form needs no
		// second pattern here.
		if !compound && reNumNext.MatchString(after[0]) {
			if _, _, end, ok := findShow(strings.ReplaceAll(s, "_", " "), false, false); ok && end > i {
				compound = true
			}
		}
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
	bracketGroup := false
	if r.Group.Value == "" {
		if m := reLastBrack.FindStringSubmatchIndex(s); m != nil {
			inner := s[m[2]:m[3]]
			// "[Thai]" at the end is a language, not the group.
			if !strings.ContainsAny(inner, " .") && !isTag(inner) && !allLangWords(inner) {
				r.Group = sure(inner)
				bracketGroup = true
				s = s[:m[0]]
			} else if strings.ContainsAny(inner, "-+") && allLangWords(inner) {
				// "[Hindi-English]": read each language on its own.
				s = s[:m[2]] + strings.NewReplacer("-", " ", "+", " ").Replace(inner) + s[m[3]:]
			}
		}
	}
	if r.Group.Value == "" && siteGroup != "" {
		r.Group = sure(siteGroup)
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
	// "[Group].Title.-.05.[1080p]": a dotted anime name reads like a
	// spaced one (same length, so the places found stay right).
	if lead != "" && !strings.Contains(strings.TrimSpace(body), " ") {
		body = strings.ReplaceAll(body, ".", " ")
	}
	sh, pre, kana, post, isShow, backwards := bracketTitleShow(body, lead != "", h.Show)
	if !isShow && !backwards {
		var start, end int
		if sh, start, end, isShow = findShow(body, lead != "", h.Show); isShow {
			pre, post = body[:start], body[end:]
		}
	}
	if isShow {
		// An anime name puts its group first; a last bracket such as
		// "[Batch]" or "[Multi-Sub]" is then not the group.
		if lead != "" && (r.Group.Conf == Unknown || bracketGroup) {
			r.Group = sure(lead)
		}
		parseShow(&r, sh, pre, post, trailing, lead != "", flags, h)
		if kana != "" {
			r.KanaTitle = kana
		}
		for f := range flags {
			r.Flags = append(r.Flags, f)
		}
		slices.Sort(r.Flags)
		return r
	}

	// "Title [2003, Country, Genre, WEB-DL 2160p]" (RuTracker): a year
	// first in square brackets ends the title, like "(2003)"; the rest of
	// that bracket is tags split by commas.
	if m := reBrackYear.FindStringSubmatchIndex(s); m != nil {
		s = s[:m[0]] + " (" + s[m[2]:m[3]] + ") " + strings.ReplaceAll(s[m[4]:m[5]], ",", " ") + s[m[1]:]
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
		if i > titleUpTo && isTag(t) && !titleTag(toks, i) {
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
	r.KanaTitle = kanaReading(toks[:end])
	title := strings.Join(latinRun(toks[:end]), " ")
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
	// MULTi.CA names a Québec French dub, so French is one of its languages.
	if flags["multi"] && !slices.Contains(r.Languages, "fr") && FrenchVersion(name) != "" {
		r.Languages = append(r.Languages, "fr")
	}

	for f := range flags {
		r.Flags = append(r.Flags, f)
	}
	slices.Sort(r.Flags)
	return r
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
		if a, b, ok := strings.Cut(t, "-"); ok && a != "" && b != "" && !tagWords[strings.ToLower(t)] && (isTag(a) || isOldAppSource(a) || strings.EqualFold(a, "hybrid")) && tagRun(b) {
			out = append(out, a)
			out = append(out, strings.Split(b, "-")...)
			continue
		}
		out = append(out, t)
	}
	return out
}

// tagRun says every dash-joined part of s is a tag ("Remux-2160p").
func tagRun(s string) bool {
	for p := range strings.SplitSeq(s, "-") {
		if p == "" || !isTag(p) && !isOldAppSource(p) {
			return false
		}
	}
	return true
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
	if tagWords[t] || reResWord.MatchString(t) || reResAny.MatchString(t) || reResPixel.MatchString(t) && len(t) >= 7 || reGlued.MatchString(t) || reDDP.MatchString(t) || reFix.MatchString(t) {
		return true
	}
	if _, ok := langWords[t]; ok {
		return true
	}
	return false
}

// ReadsLanguage reports whether Release names can show the language code at all.
func ReadsLanguage(code string) bool {
	for _, words := range []map[string]string{langWords, moreLangWords} {
		for _, c := range words {
			if c == code {
				return true
			}
		}
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

	readQuality(r, low, flags)

	// HDR. The words HolyArrs writes in a file name read back too, so a
	// Library it named keeps its quality. A written label is one word that
	// holds several kinds: DolbyVisionHDR10+, DolbyVisionHLG, HDR10HLG.
	var hdr []string
	if has("dv", "dovi", "dolbyvision", "dolbyvisionhdr10", "dolbyvisionhdr10+", "dolbyvisionhlg",
		"dolbyvisionhdr10hlg", "dolbyvisionhdr10+hlg") || strings.Contains(line, " dolby vision ") {
		hdr = append(hdr, "dv")
	}
	switch {
	case has("hdr10plus", "hdr10+", "dolbyvisionhdr10+", "dolbyvisionhdr10+hlg", "hdr10+hlg"):
		hdr = append(hdr, "hdr10plus")
	case has("hdr10", "hdr", "dolbyvisionhdr10", "dolbyvisionhdr10hlg", "hdr10hlg"):
		hdr = append(hdr, "hdr10")
	}
	if has("hlg", "dolbyvisionhlg", "dolbyvisionhdr10hlg", "dolbyvisionhdr10+hlg", "hdr10hlg", "hdr10+hlg") {
		hdr = append(hdr, "hlg")
	}
	if hdr != nil {
		r.HDR = sure(hdr)
	} else {
		r.HDR.Conf = Likely // no HDR words: SDR
	}

	// Languages. A newer language word before an ordinary word belongs to
	// an episode title or a subtitle note ("The Chinese Restaurant", "Thai Sub").
	// Toloka style: "2xUkr/Eng | Sub Ukr/Eng" lists audio languages joined
	// by "/" (a count in front), then subtitle languages after "Sub".
	var words []string // "eng|sub" reads like "eng | sub"
	for _, t := range tags {
		for j, p := range strings.Split(t, "|") {
			if j > 0 {
				words = append(words, "|")
			}
			if p != "" {
				words = append(words, p)
			}
		}
	}
	isSub := func(t string) bool {
		t = strings.ToLower(t)
		return t == "sub" || t == "subs" || t == "subtitle" || t == "subtitles"
	}
	var langToks []string // original case: upper-case-only codes need it
	for i, t := range words {
		if isSub(t) {
			if i > 0 && words[i-1] == "|" {
				break // Toloka: subtitle languages follow
			}
			// "ENG.SUBS", "Kor.Eng.Sub": the languages right before are
			// subtitles. Reading goes on after the sub word, like Sonarr.
			for len(langToks) > 0 && isLangWord(langToks[len(langToks)-1]) {
				langToks = langToks[:len(langToks)-1]
			}
			langToks = append(langToks, t) // keeps "Thai Sub 1080p" a note too
			continue
		}
		if t == "|" {
			continue
		}
		for _, p := range strings.Split(t, "/") {
			if m := reLangCount.FindStringSubmatch(p); m != nil {
				p = m[1]
			}
			langToks = append(langToks, p)
		}
	}
	lowToks := make([]string, len(langToks))
	for i, t := range langToks {
		lowToks[i] = strings.ToLower(t)
	}
	addLang := func(code string) {
		if !slices.Contains(r.Languages, code) {
			r.Languages = append(r.Languages, code)
		}
	}
	for i, t := range lowToks {
		code, ok := langWords[t]
		if !ok {
			code, ok = moreLangWords[t]
			ok = ok && (i+1 == len(lowToks) || !plainWord(lowToks[i+1]) || subTag(lowToks[i+1]))
			// "hun" also needs a tag before it, or one after when it comes
			// first, so "The.Hun.720p" stays a title.
			ok = ok && (t != "hun" || i > 0 && !plainWord(lowToks[i-1]) || i == 0 && i+1 < len(lowToks))
		}
		if !ok {
			// Upper-case two-letter codes, never next to a sub word.
			code, ok = upperLangCodes[langToks[i]]
			ok = ok && (i == 0 || !isSub(lowToks[i-1])) && (i+1 == len(lowToks) || !isSub(lowToks[i+1]))
		}
		if ok {
			addLang(code)
		}
	}
	// ORIG / ORIGINAL: the original language is in the Release too.
	if has("orig", "original") {
		flags["original"] = true
	}
	// German only: ML adds the original language and English; DL (not
	// after WEB) adds the original language.
	if len(r.Languages) == 1 && r.Languages[0] == "de" {
		if has("ml") {
			flags["original"] = true
			addLang("en")
		}
		for i, t := range low {
			if t == "dl" && (i == 0 || low[i-1] != "web") {
				flags["original"] = true
			}
		}
	}
	if has("multi", "multi-audio", "multi-subs") {
		flags["multi"] = true
	}
	// The Source says what MULTi means there; Multi-Subs is about subtitles.
	if has("multi", "multi-audio") {
		for _, code := range h.MultiLanguages {
			addLang(code)
		}
		// The Source's list says what this MULTi holds, so its languages are known, not open.
		if len(h.MultiLanguages) > 0 {
			flags["multi_known"] = true
		}
	}
	// A name that shows no language carries what its Source says every
	// release there has (before the original-language fallback).
	if len(r.Languages) == 0 {
		for _, code := range h.NoLanguage {
			addLang(code)
		}
	}
	// DL is dual language, except after WEB, where it is the source ("WEB.DL").
	for i, t := range low {
		if t == "dual" || t == "dual-audio" || t == "dl" && (i == 0 || low[i-1] != "web") {
			flags["dual"] = true
		}
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
	// REAL marks a fix of a fix ("PROPER.REAL"), so it counts one more.
	if has("real") {
		r.Fix++
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
	set("upscaled", "upscaled", "upscale", "aius", "uprez", "upsuhd", "upscaleduhd", "regrade", "regraded", "re-graded",
		"aienhanced", "aienhance", "aiupscaled", "aiupscale", "ai-enhanced", "ai-enhance", "ai-upscaled", "ai-upscale", "theupscaler")
	// Two-word tags, and a bare "AI" after the title on an HEVC encode:
	// AI upscalers tag their 4K work that way.
	for _, w := range []string{" ai enhanced ", " ai enhance ", " ai upscaled ", " ai upscale ", " the upscaler "} {
		if strings.Contains(line, w) {
			flags["upscaled"] = true
		}
	}
	if has("ai") && has("hevc", "x265", "h265") {
		flags["upscaled"] = true
	}
	set("dubbed", "dubbed")
	set("internal", "internal")
	set("hybrid", "hybrid")
	_ = h
}

// allLangWords reports whether every "-" or "+" part of s is a language
// word, so "[Hindi-English]" at the end reads as languages too.
func allLangWords(s string) bool {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '+' })
	for _, p := range parts {
		if !isLangWord(p) {
			return false
		}
	}
	return len(parts) > 0
}

// subTag says t is a joined subtitle tag such as "swesub" or "nlsubs", so
// "HUN.SWESUB" still reads Hungarian.
func subTag(t string) bool {
	return len(t) > 4 && (strings.HasSuffix(t, "sub") || strings.HasSuffix(t, "subs"))
}
