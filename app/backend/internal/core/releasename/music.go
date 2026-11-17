package releasename

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	KindAlbum       = "album"       // one album: Artist - Album (Year) [FLAC]
	KindDiscography = "discography" // many albums of one artist in one Release
)

// maxMusicName bounds the part of a name that is read; longer names are
// cut (a hostile 200 kB name costs the same as a normal one).
const maxMusicName = 2048

// maxSafetyScan bounds the safety-word scan: a longer name is scanned at
// its start and its end (each half of the bound).
const maxSafetyScan = 1 << 20

var (
	reMusicDepthRate     = regexp.MustCompile(`\b(16|24|32) ?[-/ ] ?(44\.1|44|48|88\.2|88|96|176\.4|176|192|352\.8|384)\b`)
	reMusicDepth         = regexp.MustCompile(`\b(16|24|32) ?-?bits?\b`)
	reMusicRate          = regexp.MustCompile(`\b(44\.1|44|48|88\.2|88|96|176\.4|176|192|352\.8|384) ?khz\b`)
	reMusicDiscRange     = regexp.MustCompile(`\b(?:cd|dis[ck]) ?0?(\d{1,2}) ?- ?0?(\d{1,2})\b`)
	reMusicDiscWord      = regexp.MustCompile(`^(?:cd|dis[ck])0?\d{1,2}(?:-0?\d{1,2})?$`)
	reMusicDisc          = regexp.MustCompile(`\b(?:cd|dis[ck]) ?0?(\d{1,2})\b`)
	reMusicDiscCount     = regexp.MustCompile(`\b(\d{1,2}) ?(?:cd|cds|disc|discs|disk|disks)\b`)
	reMusicDiscCountWord = regexp.MustCompile(`^\d{1,2}(?:cd|cds|disc|discs|disk|disks)$`)
	reMusicYearRange     = regexp.MustCompile(`\b((?:19|20)\d\d) ?- ?((?:19|20)\d\d)\b`)
	reMusicKbps          = regexp.MustCompile(`^(\d{2,3}) ?(?:kbps|kbit|kbits|k)$`)
	reMusicWord          = regexp.MustCompile(`[a-z0-9]+(?:\.[0-9]+)?`)
)

var musicFormats = map[string]struct {
	format   string
	lossless bool
}{
	"flac": {"flac", true}, "alac": {"alac", true}, "wav": {"wav", true}, "wave": {"wav", true}, "ape": {"ape", true},
	"wv": {"wavpack", true}, "wavpack": {"wavpack", true}, "dsd": {"dsd", true}, "dsf": {"dsd", true}, "dff": {"dsd", true},
	"mp3": {"mp3", false}, "aac": {"aac", false}, "m4a": {"aac", false}, "ogg": {"vorbis", false}, "vorbis": {"vorbis", false},
	"opus": {"opus", false}, "wma": {"wma", false}, "pcm": {"pcm", true}, "tta": {"tta", true},
	"aiff": {"aiff", true}, "aif": {"aiff", true},
}

var musicSources = map[string]string{
	"cd": "cd", "cdrip": "cd", "cdda": "cd",
	"web": "web", "webflac": "web", "webrip": "web", "webdl": "web", "hdtracks": "web", "qobuz": "web", "bandcamp": "web",
	"itunes": "web", "deezer": "web", "tidal": "web", "amazon": "web", "7digital": "web", "beatport": "web",
	"vinyl": "vinyl", "lp": "vinyl", "vinylrip": "vinyl",
	"sacd": "sacd", "cassette": "cassette", "tape": "cassette", "dvda": "dvd",
}

var musicEditions = map[string]string{
	"deluxe": "deluxe", "remaster": "remaster", "remastered": "remaster", "remasters": "remaster", "live": "live",
	"bonus": "bonus", "expanded": "expanded", "anniversary": "anniversary", "reissue": "reissue", "limited": "limited",
	"special": "special", "japan": "japan", "japanese": "japan", "edition": "edition", "collectors": "special",
}

var musicBitrates = map[int]bool{32: true, 48: true, 64: true, 80: true, 96: true, 112: true, 128: true, 160: true,
	192: true, 224: true, 256: true, 320: true}

var musicOther = setOf("vbr", "cbr", "v0", "v1", "v2", "apx", "aps", "kbps", "kbit", "log", "cue", "lossless", "hires",
	"proper", "repack", "scene", "discography", "discografia", "diskografie", "image", "img", "transcode", "transcoded",
	"sample", "password", "passworded", "khz", "bit", "24bit", "16bit")

// MusicInfo is what an album Release name says.
type MusicInfo struct {
	Artist string `json:"artist"`
	Album  string `json:"album,omitempty"`
	// Alts are other readings of the same name: a scene name whose artist
	// holds a hyphen ("Jay-Z-Album-...") or a name with no clear split.
	Alts       []MusicAlt `json:"alts,omitempty"`
	Format     string     `json:"format,omitempty"` // flac, alac, wav, ape, wavpack, dsd, mp3, aac, vorbis, opus, wma
	Lossless   bool       `json:"lossless,omitempty"`
	Bitrate    int        `json:"bitrate,omitempty"`     // kbps
	VBR        string     `json:"vbr,omitempty"`         // V0, V1, V2 or VBR
	BitDepth   int        `json:"bit_depth,omitempty"`   // 16, 24, 32
	SampleRate float64    `json:"sample_rate,omitempty"` // kHz
	Source     string     `json:"source,omitempty"`      // cd, web, vinyl, sacd, cassette, dvd
	Editions   []string   `json:"editions,omitempty"`    // deluxe, remaster, live, bonus, expanded, anniversary, reissue, limited, special, japan, edition
	Discs      []int      `json:"discs,omitempty"`       // CD2 names disc 2 only
	DiscCount  int        `json:"disc_count,omitempty"`  // 2CD
	YearTo     int        `json:"year_to,omitempty"`     // the last year of a discography
	Various    bool       `json:"various,omitempty"`     // Various Artists (VA)
	Scene      bool       `json:"scene,omitempty"`       // Artist-Album-TAGS-YEAR-GROUP
	Conf       Conf       `json:"conf"`
}

// MusicAlt is another reading of a music name.
type MusicAlt struct {
	Artist string `json:"artist"`
	Album  string `json:"album"`
}

// IsMusic says the name was read as music.
func (r *Result) IsMusic() bool { return r.Music != nil }

// musicTagWord says a lower-case word is a music tag (not part of a name).
func musicTagWord(w string) bool {
	if _, ok := musicFormats[w]; ok {
		return true
	}
	if _, ok := musicSources[w]; ok {
		return true
	}
	if _, ok := musicEditions[w]; ok {
		return true
	}
	if musicOther[w] || reMusicKbps.MatchString(w) {
		return true
	}
	if n, err := strconv.Atoi(w); err == nil && musicBitrates[n] && n >= 128 {
		return true
	}
	return false
}

func isYearWord(w string) bool {
	if !reYear.MatchString(w) {
		return false
	}
	y, _ := strconv.Atoi(w)
	return y <= nextYear
}

func hasYearWord(s string) bool {
	return slices.ContainsFunc(reMusicWord.FindAllString(s, -1), isYearWord)
}

// musicTagText says a piece of a name is tags: a year, or a tag word.
func musicTagText(s string) bool {
	l := strings.ToLower(s)
	if reMusicYearRange.MatchString(l) || reMusicDepth.MatchString(l) || reMusicRate.MatchString(l) || reMusicDepthRate.MatchString(l) ||
		reMusicDisc.MatchString(l) || reMusicDiscCount.MatchString(l) {
		return true
	}
	for _, w := range reMusicWord.FindAllString(l, -1) {
		if isYearWord(w) || musicTagWord(w) {
			return true
		}
	}
	return false
}

// musicAllTags says every word of a piece of a name is a tag, a year or a
// number ("CD", "FLAC 2019", "24-96"): a scene segment or a " - " piece that
// is not more of the name.
func musicAllTags(s string) bool {
	ws := reMusicWord.FindAllString(strings.ToLower(s), -1)
	if len(ws) == 0 {
		return false
	}
	for _, w := range ws {
		if isYearWord(w) || musicTagWord(w) || reNumber(strings.ReplaceAll(w, ".", "")) {
			continue
		}
		if reMusicDepth.MatchString(w) || reMusicRate.MatchString(w) || reMusicDisc.MatchString(w) || reMusicDiscCount.MatchString(w) {
			continue
		}
		return false
	}
	return true
}

// musicNormalise turns underscores, and dots used as spaces, into spaces;
// a dot between digits (44.1) stays.
func musicNormalise(s string) string {
	dotsAreSpaces := !strings.Contains(s, " ")
	b := []byte(s)
	for i, c := range b {
		switch {
		case c == '_':
			b[i] = ' '
		case c == '.' && dotsAreSpaces:
			if i > 0 && i < len(b)-1 && b[i-1] >= '0' && b[i-1] <= '9' && b[i+1] >= '0' && b[i+1] <= '9' {
				continue
			}
			b[i] = ' '
		}
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

// parseMusic reads an album Release name (pick-rules.md 2.5 step 6).
func parseMusic(name string, h Hints) Result {
	r := Result{Original: name, Kind: sure(KindAlbum)}
	r.Year.Conf, r.Resolution.Conf, r.Source.Conf, r.Codec.Conf, r.HDR.Conf, r.Group.Conf = Unknown, Unknown, Unknown, Unknown, Unknown, Unknown
	m := &MusicInfo{Conf: Unknown}
	r.Music = m
	flags := map[string]bool{}
	s := strings.TrimSpace(name)
	if loc := reSitePrefix.FindStringIndex(s); loc != nil {
		s = s[loc[1]:]
		flags["site_prefix"] = true
	}
	// Extensions at the end: a program after an audio or video one hides a
	// program. Format characters (zero-width) and trailing dots and spaces
	// are dropped first so they cannot hide one.
	s = musicTrimEnd(dropFormatRunes(s))
	sawExe := false
	for {
		i := strings.LastIndexByte(s, '.')
		if i < 0 {
			break
		}
		ext := strings.ToLower(s[i+1:])
		f, audio := musicFormats[ext]
		switch {
		case exeExt[ext]:
			sawExe = true
		case videoExt[ext]:
			if sawExe {
				flags["double_extension"] = true
			}
		case audio && ext != "ape" && ext != "wv":
			if sawExe {
				flags["double_extension"] = true
			}
			if m.Format == "" {
				m.Format, m.Lossless = f.format, f.lossless
			}
		default:
			i = -1
		}
		if i < 0 {
			break
		}
		s = musicTrimEnd(s[:i])
	}
	if sawExe {
		flags["executable"] = true
	}
	// Safety words count wherever they are in the name, the album part too,
	// and past the cut below: the whole name is scanned first.
	musicSafetyWords(s, flags)
	for _, f := range h.FileList {
		lf := strings.ToLower(musicTrimEnd(dropFormatRunes(f)))
		if j := strings.LastIndexByte(lf, '.'); j >= 0 && exeExt[lf[j+1:]] {
			flags["executable"] = true
		}
		musicFileSafety(f, flags)
	}
	// The extensions above are read on the whole name; only then is a very
	// long name cut, so a program extension past the cut is still seen.
	if len(s) > maxMusicName {
		s = strings.ToValidUTF8(s[:maxMusicName], "")
		for len(s) > 0 && !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
		r.Notes = append(r.Notes, "the name is very long; only its first 2048 characters were read")
	}
	s = musicNormalise(s)

	var artist, album string
	var tags []string
	if i := strings.Index(s, " - "); i > 0 {
		artist = strings.TrimSpace(s[:i])
		rest := s[i+3:]
		// A trailing -GROUP right after a bracket: "... [FLAC]-GRP".
		if j := strings.LastIndexByte(rest, '-'); j > 0 && j < len(rest)-1 && (rest[j-1] == ']' || rest[j-1] == ')') &&
			!strings.ContainsAny(rest[j+1:], " []()") && !musicTagWord(strings.ToLower(rest[j+1:])) {
			r.Group = sure(rest[j+1:])
			rest = rest[:j]
		}
		segs := strings.Split(rest, " - ")
		album, tags = musicAlbumRegion(segs[0])
		for _, seg := range segs[1:] {
			if len(tags) == 0 && !musicAllTags(seg) {
				// More of the album name: "Album - Part Two".
				a, t := musicAlbumRegion(seg)
				album = strings.TrimSpace(album + " - " + a)
				tags = t
				continue
			}
			tags = append(tags, seg)
		}
		m.Conf = Sure
	} else if segs := splitScene(s); len(segs) >= 2 {
		m.Scene = true
		if last := segs[len(segs)-1]; len(segs) >= 3 && !strings.ContainsAny(last, " ()[]") && !musicAllTags(last) {
			r.Group = sure(last)
			segs = segs[:len(segs)-1]
		}
		k := 1
		for k < len(segs) && !musicAllTags(segs[k]) && !strings.HasPrefix(segs[k], "(") {
			k++
		}
		// A number right after the artist is the album ("Adele-25-WEB-2015",
		// "Taylor_Swift-1989-WEB-2014") when it is not a year, or a later
		// segment gives the year.
		if k == 1 && len(segs) >= 3 && reNumber(segs[1]) && (!isYearWord(segs[1]) || slices.ContainsFunc(segs[2:], hasYearWord)) {
			k = 2
		}
		artist = segs[0]
		if k > 1 {
			album = strings.Join(segs[1:k], " - ")
		}
		if k >= 3 {
			m.Alts = append(m.Alts, MusicAlt{Artist: segs[0] + "-" + segs[1], Album: strings.Join(segs[2:k], " - ")})
		}
		for _, seg := range segs[k:] {
			if strings.HasPrefix(seg, "(") && strings.HasSuffix(seg, ")") && !musicAllTags(seg) {
				continue // a catalogue number
			}
			tags = append(tags, seg)
		}
		m.Conf = Likely
	} else {
		// No clear split: the words before the tags, read as artist and
		// album together; the matcher tries every split.
		a, t := musicAlbumRegion(s)
		artist, tags = a, t
		m.Conf = Guess
	}
	artist = strings.TrimSpace(artist)
	album = strings.TrimSpace(album)
	switch strings.ToLower(strings.Trim(artist, ". ")) {
	case "va", "v a", "v.a", "various", "various artists":
		m.Various = true
	}
	if l := CleanName(album); l == "discography" || l == "discografia" || l == "diskografie" || strings.HasSuffix(l, " discography") ||
		strings.HasPrefix(l, "discography ") || l == "complete discography" {
		r.Kind = sure(KindDiscography)
		album = ""
	}
	m.Artist, m.Album = artist, album
	r.Title = Field[string]{artist, m.Conf}
	if artist == "" {
		r.Title.Conf = Unknown
	}
	readMusicTags(&r, m, strings.ToLower(strings.Join(tags, " ")), flags)
	if m.Format != "" {
		r.Audio.Format = m.Format
	}
	for f := range flags {
		r.Flags = append(r.Flags, f)
	}
	slices.Sort(r.Flags)
	return r
}

func isFormatRune(r rune) bool { return unicode.Is(unicode.Cf, r) }

// dropFormatRunes removes format characters (zero-width spaces, soft
// hyphens, direction marks) that can split a word or an extension.
func dropFormatRunes(s string) string {
	if strings.IndexFunc(s, isFormatRune) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isFormatRune(r) {
			return -1
		}
		return r
	}, s)
}

// musicTrimEnd drops trailing dots, spaces and format characters.
func musicTrimEnd(s string) string {
	return strings.TrimRightFunc(s, func(r rune) bool { return r == '.' || unicode.IsSpace(r) || isFormatRune(r) })
}

// foldSafety turns text into the lower-case words a safety scan reads:
// format characters removed, then NFKC (full-width letters become plain).
// It is linear and bounded by maxSafetyScan.
func foldSafety(s string) string {
	if len(s) > maxSafetyScan {
		h := maxSafetyScan / 2
		s = strings.ToValidUTF8(s[:h], "") + " " + strings.ToValidUTF8(s[len(s)-h:], "")
	}
	return strings.ToLower(norm.NFKC.String(dropFormatRunes(s)))
}

// musicSafetyWords flags password, sample and transcode words anywhere in s.
func musicSafetyWords(s string, flags map[string]bool) {
	for _, w := range reMusicWord.FindAllString(foldSafety(s), -1) {
		switch w {
		case "password", "passworded":
			flags["password_hint"] = true
		case "sample":
			flags["sample"] = true
		case "transcode", "transcoded":
			flags["transcode"] = true
		}
	}
}

// musicFileSafety reads one file of the file list: password and transcode
// words anywhere; a sample only as a folder or file named for it ("Sample/",
// "sample.flac"), since a track title may hold the word.
func musicFileSafety(f string, flags map[string]bool) {
	l := foldSafety(f)
	for _, w := range reMusicWord.FindAllString(l, -1) {
		switch w {
		case "password", "passworded":
			flags["password_hint"] = true
		case "transcode", "transcoded":
			flags["transcode"] = true
		}
	}
	for _, part := range strings.FieldsFunc(l, func(r rune) bool { return r == '/' || r == '\\' }) {
		if ws := reMusicWord.FindAllString(part, 2); len(ws) > 0 && ws[0] == "sample" {
			flags["sample"] = true
		}
	}
}

// splitScene splits a scene name on its hyphens ("Artist-Album-WEB-2024-GRP");
// a hyphen inside brackets or between a bit depth and a rate (24-96) stays.
func splitScene(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case '-':
			if depth > 0 || i > 0 && i < len(s)-1 && unicode.IsDigit(rune(s[i-1])) && unicode.IsDigit(rune(s[i+1])) {
				continue
			}
			if seg := strings.TrimSpace(s[start:i]); seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	if seg := strings.TrimSpace(s[start:]); seg != "" {
		out = append(out, seg)
	}
	return out
}

// musicAlbumRegion splits "Album (Deluxe Edition) (2021) [FLAC]" into the
// album name and the tags after it. A bracket of plain words before any tag
// stays in the name ("Album (Part One)"); a year that is not the first word,
// a format, a bit rate or a source word with tags after it ends the name.
func musicAlbumRegion(s string) (string, []string) {
	var name []string
	var tags []string
	stopped := false
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '(' || c == '[' || c == '{' {
			close := map[byte]byte{'(': ')', '[': ']', '{': '}'}[c]
			j := strings.IndexByte(s[i+1:], close)
			if j < 0 {
				j = len(s) - i - 1
			}
			inner := s[i+1 : i+1+j]
			if stopped || musicTagText(inner) {
				stopped = true
				tags = append(tags, inner)
			} else {
				name = append(name, string(c)+inner+string(close))
			}
			i += j + 2
			continue
		}
		j := strings.IndexAny(s[i:], "([{")
		if j < 0 {
			j = len(s) - i
		}
		text := s[i : i+j]
		i += j
		if stopped {
			tags = append(tags, text)
			continue
		}
		words := strings.Fields(text)
		for k, w := range words {
			lw := strings.ToLower(strings.Trim(w, ",;:"))
			first := len(name) == 0 && k == 0
			_, isSource := musicSources[lw]
			_, isFormat := musicFormats[lw]
			last := k == len(words)-1
			// A disc count ("2CD") is a tag wherever it stands after the first word.
			stop := !first && reMusicDiscWord.MatchString(lw) && (last || musicTagWord(strings.ToLower(words[k+1]))) ||
				!first && reMusicDiscCountWord.MatchString(lw) ||
				!first && isYearWord(lw) || isFormat || reMusicKbps.MatchString(lw) || reMusicDepth.MatchString(lw) ||
				isSource && !first && (last || musicTagWord(strings.ToLower(words[k+1])) || isYearWord(strings.ToLower(words[k+1])))
			if stop {
				stopped = true
				tags = append(tags, strings.Join(words[k:], " "))
				break
			}
			name = append(name, w)
		}
	}
	return strings.Join(name, " "), tags
}

// readMusicTags reads format, bit rate, bit depth, sample rate, source,
// editions, discs, years and flags from the lower-case tag text.
func readMusicTags(r *Result, m *MusicInfo, t string, flags map[string]bool) {
	cut := func(re *regexp.Regexp, f func([]string)) {
		for _, x := range re.FindAllStringSubmatch(t, 8) {
			f(x)
		}
		t = re.ReplaceAllString(t, " ")
	}
	cut(reMusicYearRange, func(x []string) {
		a, _ := strconv.Atoi(x[1])
		b, _ := strconv.Atoi(x[2])
		if r.Year.Value == 0 && a <= b {
			r.Year, m.YearTo = sure(a), b
		}
	})
	rate := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		switch v {
		case 44:
			return 44.1
		case 88:
			return 88.2
		case 176:
			return 176.4
		}
		return v
	}
	cut(reMusicDepthRate, func(x []string) {
		m.BitDepth, _ = strconv.Atoi(x[1])
		m.SampleRate = rate(x[2])
	})
	cut(reMusicDepth, func(x []string) { m.BitDepth, _ = strconv.Atoi(x[1]) })
	cut(reMusicRate, func(x []string) { m.SampleRate = rate(x[1]) })
	cut(reMusicDiscRange, func(x []string) {
		a, _ := strconv.Atoi(x[1])
		b, _ := strconv.Atoi(x[2])
		for n := a; n > 0 && a < b && b-a < 20 && n <= b; n++ {
			if !slices.Contains(m.Discs, n) {
				m.Discs = append(m.Discs, n)
			}
		}
	})
	cut(reMusicDiscCount, func(x []string) { m.DiscCount, _ = strconv.Atoi(x[1]) })
	cut(reMusicDisc, func(x []string) {
		if n, _ := strconv.Atoi(x[1]); n > 0 && !slices.Contains(m.Discs, n) {
			m.Discs = append(m.Discs, n)
		}
	})
	toks := reMusicWord.FindAllString(t, -1)
	for k, w := range toks {
		next := ""
		if k+1 < len(toks) {
			next = toks[k+1]
		}
		if f, ok := musicFormats[w]; ok {
			if m.Format == "" || f.lossless && !m.Lossless {
				m.Format, m.Lossless = f.format, f.lossless
			}
			continue
		}
		if src, ok := musicSources[w]; ok {
			if m.Source == "" {
				m.Source = src
			}
			continue
		}
		if e, ok := musicEditions[w]; ok {
			if !slices.Contains(m.Editions, e) {
				m.Editions = append(m.Editions, e)
			}
			continue
		}
		switch w {
		case "v0", "apx":
			m.VBR = "V0"
		case "v1":
			m.VBR = "V1"
		case "v2", "aps":
			m.VBR = "V2"
		case "vbr":
			if m.VBR == "" {
				m.VBR = "VBR"
			}
		case "lossless":
			m.Lossless = true
		case "hires":
			if m.BitDepth == 0 {
				m.BitDepth = 24
			}
		case "log", "cue":
			flags[w] = true
		case "image", "img":
			flags["image"] = true
		case "transcode", "transcoded":
			flags["transcode"] = true
		case "sample":
			flags["sample"] = true
		case "password", "passworded":
			flags["password_hint"] = true
		case "proper", "repack":
			r.Fix = max(r.Fix, 1)
		case "discography", "discografia", "diskografie":
			r.Kind = sure(KindDiscography)
		}
		if mm := reMusicKbps.FindStringSubmatch(w); mm != nil {
			m.Bitrate, _ = strconv.Atoi(mm[1])
			continue
		}
		if n, err := strconv.Atoi(w); err == nil {
			switch {
			case musicBitrates[n] && (next == "kbps" || next == "kbit" || next == "k"):
				m.Bitrate = n
			case musicBitrates[n] && n >= 128 && m.Bitrate == 0:
				m.Bitrate = n
			case isYearWord(w) && r.Year.Value == 0:
				r.Year = sure(n)
			}
		}
	}
	if flags["image"] && flags["cue"] {
		flags["single_image"] = true
	}
	delete(flags, "image")
	if m.Format == "" && m.Lossless {
		m.Format = "flac" // "Lossless" with no format word: FLAC is by far the most common
	}
	switch m.Source {
	case "cd":
		r.Source = sure("cd")
	case "vinyl":
		r.Source = sure("vinyl")
	case "web":
		r.Source = sure("web_music")
	case "":
	default:
		r.Source = sure(m.Source)
	}
	if r.Year.Value == 0 {
		r.Year.Conf = Unknown
	}
}
