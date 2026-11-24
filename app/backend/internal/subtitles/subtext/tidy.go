package subtext

import (
	"regexp"
	"strings"
	"unicode"
)

// Tidy picks the cleanup steps run on a saved subtitle. Ads is on by default;
// the others are each turned on by the owner.
type Tidy struct {
	Ads     bool `json:"ads"`     // ad and credit lines near the start and end
	HI      bool `json:"hi"`      // hearing-impaired cues such as [door slams] and speaker names
	Tags    bool `json:"tags"`    // formatting tags such as <i> and {\an8}
	Emoji   bool `json:"emoji"`   // emoji and pictographs
	OCR     bool `json:"ocr"`     // common l / I mix-ups from disc text reading
	Caps    bool `json:"caps"`    // a file written all in capitals
	Spacing bool `json:"spacing"` // double spaces and spaces before punctuation
	Overlap bool `json:"overlap"` // cues that run into the next one
	// Language is the subtitle's language, for its ad word list.
	Language string `json:"-"`
}

// Any reports whether a step is on.
func (t Tidy) Any() bool {
	return t.Ads || t.HI || t.Tags || t.Emoji || t.OCR || t.Caps || t.Spacing || t.Overlap
}

// Steps names the steps that are on, for the History note.
func (t Tidy) Steps() []string {
	var s []string
	for _, x := range []struct {
		on   bool
		name string
	}{{t.Ads, "ads"}, {t.HI, "hi"}, {t.Tags, "tags"}, {t.Emoji, "emoji"}, {t.OCR, "ocr"}, {t.Caps, "caps"}, {t.Spacing, "spacing"}, {t.Overlap, "overlap"}} {
		if x.on {
			s = append(s, x.name)
		}
	}
	return s
}

// TidyReport says what a cleanup changed.
type TidyReport struct {
	Removed []string `json:"removed"`           // text of cues taken out as ads or credits, at most 20
	Ads     int      `json:"ads"`               // ad cues removed
	Changed int      `json:"changed"`           // cues whose text changed, were dropped or retimed by the other steps
	Skipped []string `json:"skipped,omitempty"` // steps left out for this file, and why
}

// Did reports whether anything changed.
func (r TidyReport) Did() bool { return r.Ads > 0 || r.Changed > 0 }

// adEdge is how many cues from each end are checked for ads. Ads sit at the
// start and end; the middle of a film is left alone.
const adEdge = 6

// MinGapMS keeps two frames between cues at 23.976 fps.
const MinGapMS = 84

var (
	urlRe = regexp.MustCompile(`(?i)(https?://|www\.)\S+|\b[a-z0-9-]+\.(com|net|org|io|tv|info|me|to|cc|xyz|ru|ro|gr|pl|de|fr|es|it|nl|br|co)\b`)
	// siteWords are subtitle sites and groups. Any of them alone marks an ad.
	siteWords = []string{"opensubtitles", "addic7ed", "podnapisi", "subscene", "yifysubtitles", "subdl", "osdb.link", "explosiveskull",
		"subtitlecat", "titlovi", "legendas.tv", "napiprojekt", "subf2m"}
	// adWords are credit and ad phrases per language. "" applies to every file.
	adWords = map[string][]string{
		"": {"subtitles by", "subtitle by", "subtitled by", "subs by", "synced by", "sync by", "synced and corrected", "resync by", "ripped by",
			"corrected by", "encoded by", "translated by", "translation by", "support us", "become a vip", "advertise your product",
			"downloaded from", "please rate this subtitle", "remove all ads", "watch online", "best watched using", "transcript by",
			"captioning by", "captioned by"},
		"es": {"subtítulos por", "subtitulado por", "traducido por", "traducción por", "sincronizado por", "apóyanos", "subtítulos:"},
		"pt": {"legendas por", "legenda por", "tradução por", "traduzido por", "sincronizado por", "sincronia por", "revisão por", "apoie-nos"},
		"fr": {"sous-titres par", "sous-titré par", "traduit par", "traduction par", "synchro par", "soutenez-nous"},
		"de": {"untertitel von", "untertitel:", "übersetzt von", "übersetzung von", "synchronisiert von", "unterstützt uns"},
		"it": {"sottotitoli di", "sottotitoli a cura di", "tradotto da", "traduzione di", "sincronizzato da", "sostienici"},
		"nl": {"ondertiteling door", "vertaald door", "vertaling door", "gesynchroniseerd door", "steun ons"},
		"pl": {"napisy:", "tłumaczenie:", "przetłumaczył", "synchro:", "wspieraj nas"},
		"ro": {"subtitrare:", "subtitrarea", "traducerea:", "traducere:", "sincronizare:"},
		"el": {"υπότιτλοι:", "μετάφραση:", "συγχρονισμός:"},
		"tr": {"altyazı:", "çeviri:", "çeviren:", "senkron:"},
		"sv": {"undertexter av", "översättning:", "översatt av"},
		"da": {"tekster af", "oversat af", "tekstet af"},
		"no": {"tekst av", "oversatt av", "tekstet av"},
		"nb": {"tekst av", "oversatt av", "tekstet av"},
		"fi": {"suomennos:", "tekstitys:"},
		"ru": {"субтитры:", "перевод:", "переведено", "синхронизация:"},
		"uk": {"субтитри:", "переклад:"},
		"ar": {"ترجمة:", "تعديل التوقيت"},
		"he": {"תורגם על ידי", "תרגום:", "סונכרן על ידי"},
		"cs": {"titulky:", "překlad:", "přeložil"},
		"hu": {"felirat:", "fordította", "fordítás:"},
		"id": {"diterjemahkan oleh", "terjemahan:", "subtitle oleh"},
		"vi": {"phụ đề:", "dịch bởi", "biên dịch:"},
		"zh": {"字幕：", "字幕:", "翻译：", "翻译:", "翻譯："},
		"ja": {"字幕：", "翻訳："},
		"ko": {"자막:", "번역:"},
	}
	// urlVerbs make a web address an ad: a bare address may be a line of dialogue.
	urlVerbs = []string{"visit", "download", "watch", "subscribe", "follow us", "join us", "for more", "get ", "rate "}

	hiSquare = regexp.MustCompile(`\[[^\]\n]*\]|♪+`)
	// musicHash is a # used as a music mark at a line's start or end, not "#1".
	musicHash = regexp.MustCompile(`^\s*#+(\s+|$)|(^|\s+)#+\s*$`)
	hiParen   = regexp.MustCompile(`\([^)\n]*\)`)
	speaker   = regexp.MustCompile(`^(-\s*)?([\p{Lu}][\p{Lu}0-9 .'\-]{1,30}):\s*`)
	tagRe     = regexp.MustCompile(`</?[a-zA-Z][^>\n]*>|\{\\[^}\n]*\}`)
	loneL     = regexp.MustCompile(`(^|[\s"-])l(\s)`)
	lowerI    = regexp.MustCompile(`\bi\b`)
	ocrWords  = map[string]string{"lt": "It", "lts": "Its", "lt's": "It's", "lf": "If", "ln": "In", "ls": "Is", "lsn't": "Isn't", "l'm": "I'm", "l'll": "I'll", "l've": "I've", "l'd": "I'd"}
	wordRe    = regexp.MustCompile(`[\p{L}']+`)
	spaces    = regexp.MustCompile(`[ \t]{2,}`)
	beforeP   = regexp.MustCompile(` +([,.!?;:])`)
	beforePFr = regexp.MustCompile(` +([,.])`) // French keeps its space before ? ! : ;
	notNames  = map[string]bool{"OK": true, "NO": true, "YES": true, "OH": true, "HEY": true, "NOTE": true, "PS": true}
)

func baseLang(lang string) string {
	lang = strings.ToLower(lang)
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		return lang[:i]
	}
	return lang
}

func isAd(text, lang string) bool {
	low := strings.ToLower(text)
	for _, w := range siteWords {
		if strings.Contains(low, w) {
			return true
		}
	}
	for _, l := range []string{"", baseLang(lang)} {
		for _, w := range adWords[l] {
			if strings.Contains(low, w) {
				return true
			}
		}
	}
	if urlRe.MatchString(low) {
		for _, w := range urlVerbs {
			if strings.Contains(low, w) {
				return true
			}
		}
	}
	return false
}

func isEmoji(r rune) bool {
	return r >= 0x1F000 && r <= 0x1FAFF || r >= 0x2600 && r <= 0x27BF && r != 0x266A && r != 0x266B || r == 0xFE0F || r == 0x200D
}

// mostlyCaps reports whether the file is written in capitals: at least 90 of
// every 100 cased letters outside sound notes, and at least 100 of them, so a
// few shouted lines in a normal file do not count.
func mostlyCaps(cues []Cue) bool {
	upper, lower := 0, 0
	for _, c := range cues {
		for _, r := range hiSquare.ReplaceAllString(tagRe.ReplaceAllString(c.Text, ""), "") {
			switch {
			case unicode.IsUpper(r):
				upper++
			case unicode.IsLower(r):
				lower++
			}
		}
	}
	return upper >= 100 && upper*100 >= (upper+lower)*90
}

// sentenceCase lowers a cue written in capitals and raises each sentence start.
func sentenceCase(s, lang string) string {
	var sb strings.Builder
	start := true
	for _, r := range strings.ToLower(s) {
		if start && unicode.IsLetter(r) {
			r, start = unicode.ToUpper(r), false
		}
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			start = true
		}
		sb.WriteRune(r)
	}
	if l := baseLang(lang); l != "" && l != "en" { // "i" is a word of its own in Polish, Italian, Czech...
		return sb.String()
	}
	return lowerI.ReplaceAllString(sb.String(), "I")
}

func fixOCR(s string) string {
	s = loneL.ReplaceAllString(s, "${1}I${2}")
	return wordRe.ReplaceAllStringFunc(s, func(w string) string {
		if to, ok := ocrWords[w]; ok {
			return to
		}
		return w
	})
}

// dropSpeaker takes a speaker name such as "JOHN:" off the start of a line. A
// digit right after the colon is a clock time (MEET AT 10:30), and a short
// word such as OK is not a name.
func dropSpeaker(line string) string {
	m := speaker.FindStringSubmatchIndex(line)
	if m == nil {
		return line
	}
	name := strings.TrimSpace(line[m[4]:m[5]])
	colon := m[5] // the colon sits right after the name
	if colon+1 < len(line) && line[colon+1] >= '0' && line[colon+1] <= '9' {
		return line
	}
	if notNames[name] || strings.IndexFunc(name, unicode.IsLetter) < 0 {
		return line
	}
	dash := ""
	if m[2] >= 0 {
		dash = line[m[2]:m[3]]
	}
	return dash + line[m[1]:]
}

// dropHI removes sound notes and speaker names from one line. A note in round
// brackets goes only when it is the whole line or written in capitals, so an
// aside such as "(and he agreed)" stays. A note that opens the line, such as
// "(sighs) Okay", goes too.
func dropHI(line string, names bool) string {
	line = musicHash.ReplaceAllString(hiSquare.ReplaceAllString(line, ""), "")
	lead := strings.TrimLeft(line, "- ")
	line = hiParen.ReplaceAllStringFunc(line, func(p string) string {
		inner := p[1 : len(p)-1]
		if strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "- ")) == p || strings.HasPrefix(lead, p) || (strings.ToUpper(inner) == inner && strings.IndexFunc(inner, unicode.IsLetter) >= 0) {
			return ""
		}
		return p
	})
	if names {
		line = dropSpeaker(line)
	}
	return line
}

// CleanCues runs the chosen steps over cues and returns the kept cues.
func CleanCues(cues []Cue, t Tidy) ([]Cue, TidyReport) {
	var rep TidyReport
	caps := len(cues) > 0 && (t.HI || t.Caps) && mostlyCaps(cues)
	if t.HI && caps {
		rep.Skipped = append(rep.Skipped, "speaker names: the file is in capitals")
	}
	if t.Caps && !caps {
		rep.Skipped = append(rep.Skipped, "capitals: the file is not in capitals")
	}
	out := make([]Cue, 0, len(cues))
	for i, c := range cues {
		if t.Ads && (i < adEdge || i >= len(cues)-adEdge) && isAd(c.Text, t.Language) {
			rep.Ads++
			if len(rep.Removed) < 20 {
				rep.Removed = append(rep.Removed, strings.ReplaceAll(c.Text, "\n", " "))
			}
			continue
		}
		text := c.Text
		if t.Tags {
			text = tagRe.ReplaceAllString(text, "")
		}
		if t.Emoji {
			text = strings.Map(func(r rune) rune {
				if isEmoji(r) {
					return -1
				}
				return r
			}, text)
		}
		if t.OCR {
			text = fixOCR(text)
		}
		if t.Caps && caps {
			text = sentenceCase(text, t.Language)
		}
		lines := strings.Split(text, "\n")
		kept := lines[:0]
		for _, l := range lines {
			if t.HI {
				l = dropHI(l, !caps)
			}
			if t.Spacing || t.HI {
				l = strings.TrimSpace(spaces.ReplaceAllString(l, " "))
			}
			if t.Spacing {
				if baseLang(t.Language) == "fr" {
					l = beforePFr.ReplaceAllString(l, "$1")
				} else {
					l = beforeP.ReplaceAllString(l, "$1")
				}
			}
			if strings.TrimSpace(l) != "" && strings.TrimSpace(l) != "-" {
				kept = append(kept, l)
			}
		}
		text = strings.Join(kept, "\n")
		// A line that now reads as a time line would split the cue when the
		// file is read again, so the cue keeps its text.
		for _, l := range kept {
			if _, _, ok := cueTimes(strings.TrimSpace(l)); ok || strings.Contains(l, "-->") {
				text = c.Text
				break
			}
		}
		if text != c.Text {
			rep.Changed++
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		c.Text = text
		out = append(out, c)
	}
	if t.Overlap {
		for i := 0; i+1 < len(out); i++ {
			if lim := out[i+1].StartMS - MinGapMS; out[i].EndMS > lim && lim > out[i].StartMS {
				out[i].EndMS = lim
				rep.Changed++
			}
		}
	}
	return out, rep
}

// TidyText cleans SRT text. Other formats, and text with no change, come back
// as given. If the cleaned file would not read back as the same cues, the
// file is left as it was.
func TidyText(text, format string, t Tidy) (string, TidyReport, error) {
	if !t.Any() || format != SRT {
		return text, TidyReport{}, nil
	}
	cues, err := ParseCues(text, format)
	if err != nil {
		return text, TidyReport{}, err
	}
	kept, rep := CleanCues(cues, t)
	if !rep.Did() || len(kept) == 0 {
		return text, TidyReport{Skipped: rep.Skipped}, nil
	}
	out := WriteSRT(kept)
	back, err := ParseCues(out, SRT)
	if err != nil || len(back) != countText(kept) {
		return text, TidyReport{Skipped: append(rep.Skipped, "the cleaned file did not read back the same")}, nil
	}
	return out, rep, nil
}

func countText(cues []Cue) int {
	n := 0
	for _, c := range cues {
		if strings.TrimSpace(c.Text) != "" {
			n++
		}
	}
	return n
}
