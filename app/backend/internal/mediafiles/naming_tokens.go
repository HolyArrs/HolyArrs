package mediafiles

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A word in braces may be written the way Sonarr, Radarr and Lidarr write
// theirs, so a pattern copied from them (or from a TRaSH naming guide)
// saves and names files the same way:
//
//   - any case and spacing: {Title}, {Movie Title}, {Movie.CleanTitle};
//   - text inside the braces kept only when the word has a value:
//     {(Release Year)}, {[Quality Full]}, {-Release Group}, { Mediainfo AudioChannels]};
//   - :00 pads a number and :N cuts a name: {season:00}, {Episode CleanTitle:90};
//   - a name written all lower or all upper case gives its value that case,
//     and dots or underscores between its parts replace the value's spaces.
//
// Our own words written exactly as listed ({title}) keep their value as is.

// tokPre and tokSuf are the characters a word may carry before and after it
// inside its braces.
const (
	tokPre = "-._[( "
	tokSuf = "-._)] "
)

// wordAliases are the old apps' names for our words, written as a key
// (lower case, without spaces, dots, dashes or underscores).
var wordAliases = map[string]string{
	// Ids.
	"tmdbid": "tmdb", "tvdbid": "tvdb", "imdbid": "imdb",
	// Names.
	"movietitle": "title", "moviecleantitle": "title", "seriestitle": "title", "seriescleantitle": "title",
	"seriestitlewithoutyear": "title", "seriescleantitlewithoutyear": "title", "movietitlewithoutyear": "title", "moviecleantitlewithoutyear": "title",
	"movietitlethe": "sort_title", "moviecleantitlethe": "sort_title", "seriestitlethe": "sort_title", "seriescleantitlethe": "sort_title",
	"seriestitlewithoutyearthe": "sort_title", "seriescleantitlewithoutyearthe": "sort_title",
	"movietitlefirstcharacter": "title_first", "seriestitlefirstcharacter": "title_first",
	"movietitleyear": "title_year", "moviecleantitleyear": "title_year", "seriestitleyear": "title_year", "seriescleantitleyear": "title_year",
	"releaseyear": "year", "movieyear": "year", "seriesyear": "year",
	"originaltitle": "release", "originalfilename": "release",
	// Release facts.
	"editiontags": "edition", "releasegroup": "group",
	"qualityfull": "quality_full", "qualitytitle": "quality_title", "qualityproper": "proper",
	"mediainfoaudiocodec": "audio", "mediainfoaudiochannels": "audio_channels",
	"mediainfovideocodec": "video_codec", "mediainfovideobitdepth": "bit_depth",
	"mediainfovideodynamicrangetype": "hdr_type", "mediainfovideodynamicrange": "hdr_type",
	"mediainfoaudiolanguages": "audio_languages", "mediainfoaudiolanguagesall": "audio_languages",
	"mediainfo3d": "three_d", "frenchversion": "french_version", "mediainfofrenchversion": "french_version", "customformats": "scored_tags", "customformat": "scored_tags",
	// Episodes.
	"episodetitle": "episode_title", "episodecleantitle": "episode_title", "airdate": "air_date",
	// Music.
	"artistname": "artist", "artistcleanname": "artist", "artistnamethe": "artist_sort", "artistcleannamethe": "artist_sort",
	"albumtitle": "album", "albumcleantitle": "album", "albumdisambiguation": "edition", "medium": "disc",
	"trackartistname": "track_artist", "trackartistcleanname": "track_artist", "trackcleantitle": "track_title",
}

// tok is one {word} of a pattern.
type tok struct {
	pre, suf string // text kept around the value only when it has one
	word     string // our word; "" when unknown
	name     string // the name as written
	pad      int    // :00 pads a number to this many digits
	cut      int    // :N cuts a name to N characters
	sep      byte   // a dot, dash or underscore that replaces the value's spaces
	lower    bool
	upper    bool
	clean    bool // a Clean name: without apostrophes and colons
	badFmt   string
}

func wordKey(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(" ._-", r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

// nativeKeys finds our own words by their key ("episodetitle"); Words is
// complete once the package has started.
var nativeKeys = sync.OnceValue(func() map[string]string {
	m := make(map[string]string, len(Words))
	for w := range Words {
		m[wordKey(w)] = w
	}
	return m
})

func lookupWord(key string) string {
	if w, ok := nativeKeys()[key]; ok {
		return w
	}
	return wordAliases[key]
}

// parseTok reads the text between a word's braces.
func parseTok(raw string) tok {
	t := tok{}
	i := 0
	for i < len(raw) && strings.IndexByte(tokPre, raw[i]) >= 0 {
		i++
	}
	t.pre, raw = raw[:i], raw[i:]
	j := len(raw)
	for j > 0 && strings.IndexByte(tokSuf, raw[j-1]) >= 0 {
		j--
	}
	t.suf, raw = raw[j:], raw[:j]
	if name, f, ok := strings.Cut(raw, ":"); ok {
		raw = name
		switch n, err := strconv.Atoi(f); {
		case err != nil || n < 0 || len(f) > 4:
			t.badFmt = f
		case strings.Trim(f, "0") == "":
			t.pad = len(f)
		default:
			t.cut = n
		}
	}
	t.name = raw
	t.word = lookupWord(wordKey(raw))
	if t.word == "" || raw == t.word {
		return t
	}
	key := wordKey(raw)
	t.clean = strings.Contains(key, "clean")
	// A dot, dash or underscore between the name's parts replaces the
	// value's spaces ({Movie.CleanTitle}); our own word in another case
	// ({Episode_Title}) keeps them. {Air Date} keeps spaces between its numbers.
	if !strings.EqualFold(raw, t.word) {
		seps := "._-"
		if t.word == "air_date" {
			seps = " ._-"
		}
		if k := strings.IndexAny(raw, seps); k >= 0 {
			t.sep = raw[k]
		}
	}
	letters := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return r
		}
		return -1
	}, raw)
	t.lower = letters != "" && letters == strings.ToLower(letters)
	t.upper = !t.lower && letters != "" && letters == strings.ToUpper(letters) && len(letters) > 1
	return t
}

// tokValue is what t gives with vals, its outside text included; "" when
// the word has no value.
func tokValue(t tok, vals map[string]string) string {
	v := vals[t.word]
	switch {
	case t.word == "episode" && t.pad > 0 && vals[keyEpisodes] != "":
		v = vals[keyEpisodes] // {episode:00} is the number, as in S{season:00}E{episode:00}
	case t.word == "absolute" && t.pad > 0 && vals[keyAbsolute] != "":
		v = vals[keyAbsolute]
	case t.word == "title_year" && t.clean:
		v = strings.TrimSpace(vals["title"] + " " + vals["year"])
	}
	if v == "" {
		return ""
	}
	if t.clean {
		v = cleanTitle(v)
	}
	if t.pad > 0 {
		v = padNumbers(v, t.pad)
	}
	if t.cut > 0 && utf8.RuneCountInString(v) > t.cut {
		v = strings.TrimRight(string([]rune(v)[:t.cut]), " .-_,")
	}
	switch {
	case t.word == "air_date" && t.sep != 0:
		v = strings.Map(func(r rune) rune {
			if r == '-' || r == '.' || r == ' ' {
				return rune(t.sep)
			}
			return r
		}, v)
	case t.sep != 0:
		v = strings.Join(strings.Fields(v), string(t.sep))
	}
	switch {
	case t.lower:
		v = strings.ToLower(v)
	case t.upper:
		v = strings.ToUpper(v)
	}
	return t.pre + v + t.suf
}

// Value keys that hold plain numbers for :00 padding.
const (
	keyEpisodes = "\x00episodes"
	keyAbsolute = "\x00absolute"
)

var reDigits = regexp.MustCompile(`\d+`)

// padNumbers pads every number in v to n digits: "1" -> "01", "12-13" -> "012-013".
func padNumbers(v string, n int) string {
	return reDigits.ReplaceAllStringFunc(v, func(d string) string {
		x, err := strconv.Atoi(d)
		if err != nil {
			return d
		}
		return fmt.Sprintf("%0*d", n, x)
	})
}

// cleanTitle drops the apostrophes, quotes and colons a Clean name leaves out:
// "Dune: Part Two" -> "Dune Part Two".
func cleanTitle(v string) string {
	v = strings.Map(func(r rune) rune {
		switch r {
		case '\'', '’', '"', ':', '?', '*', '<', '>', '|':
			return -1
		}
		return r
	}, v)
	return strings.Join(strings.Fields(v), " ")
}

// reSeasonEpisode is the old apps' episode code, which {episode} writes with
// the multi-episode style.
var reSeasonEpisode = regexp.MustCompile(`S\{season:00\}E\{episode:00\}`)

// oldEpisodeCode turns S{season:00}E{episode:00} into {episode}, so a file
// holding several episodes is named S01E02-E03 as the old apps name it.
func oldEpisodeCode(pat string) string { return reSeasonEpisode.ReplaceAllString(pat, "{episode}") }

func numbers(ns []int) string {
	parts := make([]string, 0, len(ns))
	for _, n := range ns {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, "-")
}

// Upstream-style facts.

var (
	reRepack  = regexp.MustCompile(`(?i)\b(repack|rerip)\d?\b`)
	reBits    = regexp.MustCompile(`(?i)\b(8|10|12)[ ._-]?bits?\b`)
	reThreeD  = regexp.MustCompile(`(?i)\b(3d|h-?sbs|h-?ou)\b`)
	reX26     = regexp.MustCompile(`(?i)\bx26([45])\b`)
	reWebRip  = regexp.MustCompile(`(?i)\bweb-?rip\b`)
	codecName = map[string]string{"h264": "h264", "h265": "h265", "av1": "AV1", "xvid": "XviD", "vc1": "VC1", "mpeg2": "MPEG2"}
)

// releaseFacts fills the facts a release name gives besides quality.
func releaseFacts(f *NameFacts, p releasename.Result) {
	name := p.Original
	if p.Fix > 0 {
		f.Proper = "Proper"
		if reRepack.MatchString(name) {
			f.Proper = "Repack"
		}
	}
	if m := reX26.FindStringSubmatch(name); m != nil {
		f.VideoCodec = "x26" + m[1]
	} else {
		f.VideoCodec = codecName[p.Codec.Value]
	}
	f.Channels = p.Audio.Channels
	if m := reBits.FindStringSubmatch(name); m != nil {
		f.BitDepth = m[1]
	}
	if reThreeD.MatchString(name) {
		f.ThreeD = "3D"
	}
	f.WebRip = reWebRip.MatchString(name)
	langs := p.AudioLanguages
	if len(langs) == 0 {
		langs = p.Languages
	}
	f.Languages = langs
}

// qualityTitle names a quality step the way the old apps do: Bluray-1080p,
// WEBDL-2160p, Remux-2160p, HDTV-720p, DVD, SDTV.
func qualityTitle(step string, webRip bool) string {
	res, src, ok := strings.Cut(step, "_")
	if !ok {
		return strings.ToUpper(step)
	}
	switch src {
	case "bluray":
		src = "Bluray"
	case "web":
		src = "WEBDL"
		if webRip {
			src = "WEBRip"
		}
	case "remux":
		src = "Remux"
	case "tv":
		src = "HDTV"
		if res == "sd" {
			return "SDTV"
		}
	case "dvd":
		return "DVD"
	default:
		src = strings.ToUpper(src)
	}
	if res == "sd" {
		return src + "-480p"
	}
	return src + "-" + res
}

// hdrType names HDR the way the old apps do: DV HDR10, HDR10Plus, HLG.
func hdrType(label string) string {
	label = strings.ReplaceAll(label, "DolbyVision", "DV ")
	label = strings.ReplaceAll(label, "HDR10+", "HDR10Plus ")
	label = strings.ReplaceAll(label, "HDR10", "HDR10 ")
	label = strings.ReplaceAll(label, "HDR10 Plus", "HDR10Plus")
	label = strings.ReplaceAll(label, "HLG", "HLG ")
	return strings.Join(strings.Fields(label), " ")
}

// audioLanguages writes the audio languages as [JA+EN]; English alone gives
// nothing, as in the old apps.
func audioLanguages(langs []string) string {
	var out []string
	seen := map[string]bool{}
	for _, l := range langs {
		l = strings.ToUpper(strings.TrimSpace(l))
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	if len(out) == 0 || len(out) == 1 && out[0] == "EN" {
		return ""
	}
	return "[" + strings.Join(out, "+") + "]"
}
