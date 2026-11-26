package subtitles

import "strings"

// language holds one language: its ISO 639-1 code, its ISO 639-2 codes (the
// bibliographic and terminology forms, which mostly match) and its English name.
type language struct {
	two, threeB, threeT, name string
}

var languages = []language{
	{"af", "afr", "afr", "Afrikaans"}, {"ar", "ara", "ara", "Arabic"}, {"bg", "bul", "bul", "Bulgarian"},
	{"bn", "ben", "ben", "Bengali"}, {"bs", "bos", "bos", "Bosnian"}, {"ca", "cat", "cat", "Catalan"},
	{"cs", "cze", "ces", "Czech"}, {"cy", "wel", "cym", "Welsh"}, {"da", "dan", "dan", "Danish"},
	{"de", "ger", "deu", "German"}, {"el", "gre", "ell", "Greek"}, {"en", "eng", "eng", "English"},
	{"eo", "epo", "epo", "Esperanto"}, {"es", "spa", "spa", "Spanish"},
	{"es-419", "es-419", "es-419", "Spanish (Latin America)"}, {"et", "est", "est", "Estonian"},
	{"eu", "baq", "eus", "Basque"}, {"fa", "per", "fas", "Persian"}, {"fi", "fin", "fin", "Finnish"},
	{"fr", "fre", "fra", "French"},
	{"fr-ca", "fr-ca", "fr-ca", "French (Canada)"}, {"ga", "gle", "gle", "Irish"}, {"gl", "glg", "glg", "Galician"},
	{"he", "heb", "heb", "Hebrew"}, {"hi", "hin", "hin", "Hindi"}, {"hr", "hrv", "hrv", "Croatian"},
	{"hu", "hun", "hun", "Hungarian"}, {"hy", "arm", "hye", "Armenian"}, {"id", "ind", "ind", "Indonesian"},
	{"is", "ice", "isl", "Icelandic"}, {"it", "ita", "ita", "Italian"}, {"ja", "jpn", "jpn", "Japanese"},
	{"ka", "geo", "kat", "Georgian"}, {"kk", "kaz", "kaz", "Kazakh"}, {"km", "khm", "khm", "Khmer"},
	{"ko", "kor", "kor", "Korean"}, {"ku", "kur", "kur", "Kurdish"}, {"lt", "lit", "lit", "Lithuanian"},
	{"lv", "lav", "lav", "Latvian"}, {"mk", "mac", "mkd", "Macedonian"}, {"ml", "mal", "mal", "Malayalam"},
	{"mn", "mon", "mon", "Mongolian"}, {"ms", "may", "msa", "Malay"}, {"my", "bur", "mya", "Burmese"},
	{"nb", "nob", "nob", "Norwegian Bokmål"}, {"ne", "nep", "nep", "Nepali"}, {"nl", "dut", "nld", "Dutch"},
	{"nn", "nno", "nno", "Norwegian Nynorsk"}, {"no", "nor", "nor", "Norwegian"}, {"pa", "pan", "pan", "Punjabi"},
	{"pl", "pol", "pol", "Polish"}, {"pt", "por", "por", "Portuguese"}, {"pt-br", "pob", "pob", "Portuguese (Brazil)"},
	{"pt-pt", "pt-pt", "pt-pt", "Portuguese (Portugal)"},
	{"ro", "rum", "ron", "Romanian"},
	{"ru", "rus", "rus", "Russian"}, {"si", "sin", "sin", "Sinhala"}, {"sk", "slo", "slk", "Slovak"},
	{"sl", "slv", "slv", "Slovenian"}, {"sq", "alb", "sqi", "Albanian"}, {"sr", "srp", "srp", "Serbian"},
	{"sv", "swe", "swe", "Swedish"}, {"sw", "swa", "swa", "Swahili"}, {"ta", "tam", "tam", "Tamil"},
	{"te", "tel", "tel", "Telugu"}, {"th", "tha", "tha", "Thai"}, {"tl", "tgl", "tgl", "Tagalog"},
	{"tr", "tur", "tur", "Turkish"}, {"uk", "ukr", "ukr", "Ukrainian"}, {"ur", "urd", "urd", "Urdu"},
	{"uz", "uzb", "uzb", "Uzbek"}, {"vi", "vie", "vie", "Vietnamese"}, {"zh", "chi", "zho", "Chinese"},
	{"zh-hans", "chs", "zh-hans", "Chinese (Simplified)"}, {"zh-hant", "cht", "zh-hant", "Chinese (Traditional)"},
	{"zh-yue", "yue", "yue", "Chinese (Cantonese)"},
}

// variantSpellings are the other names files, sites and players use for the
// regional and script kinds of a language. A plain base code (zh, pt, es, fr)
// still means any kind of that language.
var variantSpellings = map[string][]string{
	"pt-br":   {"pb", "ptbr", "pt_br", "br_pt", "br-pt", "brazilian", "brazilian portuguese", "portuguese (brazilian)", "pt-latn-br"},
	"pt-pt":   {"ptpt", "pt_pt", "european portuguese", "portuguese (european)"},
	"zh-hans": {"zh-cn", "zh-sg", "zh-my", "zhcn", "zh_cn", "simplified chinese", "chinese simplified", "chinese (simplified)", "zh-hans-cn", "zh-hans-sg"},
	"zh-hant": {"zh-tw", "zh-hk", "zh-mo", "zhtw", "zh_tw", "big5", "traditional chinese", "chinese traditional", "chinese (traditional)", "zh-hant-tw", "zh-hant-hk"},
	"zh-yue":  {"cantonese", "zh-ca", "yue-hk", "yue-hant", "yue-hans", "chinese (cantonese)"},
	"es-419":  {"es-la", "es-mx", "es-ar", "es-co", "es-cl", "es-pe", "es-ve", "es-us", "es_419", "es_mx", "latino", "spanish (latino)", "spanish (latin america)", "latin american spanish"},
	"fr-ca":   {"fr_ca", "frca", "canadian french", "french (canadian)"},
}

var byCode = func() map[string]language {
	m := map[string]language{}
	for _, l := range languages {
		m[l.two], m[l.threeB], m[l.threeT] = l, l, l
		m[strings.ToLower(l.name)] = l
	}
	// Extra spellings that files and sites use.
	for code, spellings := range variantSpellings {
		for _, s := range spellings {
			m[s] = m[code]
		}
	}
	m["iw"] = m["he"]
	m["in"] = m["id"]
	m["english"] = m["en"]
	return m
}()

// NormLanguage turns a language code or name into its code, so "eng", "en",
// "en-US" and "English" all give "en". The kinds in variantSpellings keep their
// own code (zh-TW gives zh-hant, es-MX gives es-419); every other region or
// script is dropped. It returns "" for unknown
// languages and for the special codes und, zxx, mul and mis.
func NormLanguage(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 40 {
		return ""
	}
	if l, ok := byCode[s]; ok {
		return l.two
	}
	i := strings.IndexAny(s, "-_")
	if i <= 0 {
		return ""
	}
	base := byCode[s[:i]].two
	if strings.Contains(base, "-") {
		return base // yue-HK: the first part already names the kind
	}
	for _, sub := range strings.FieldsFunc(s[i+1:], func(r rune) bool { return r == '-' || r == '_' }) {
		if l, ok := byCode[base+"-"+sub]; ok && base != "" && strings.HasPrefix(l.two, base+"-") {
			return l.two
		}
	}
	return base
}

// baseOf returns a language code without its region: pt-br gives pt.
func baseOf(code string) string {
	base, _, _ := strings.Cut(code, "-")
	return base
}

// LangMatches reports whether a subtitle in language got meets a wish for
// want. A wish for a base language accepts its regional files, so a pt wish
// takes pt-br, while a pt-br wish needs pt-br.
func LangMatches(want, got string) bool {
	return want != "" && (want == got || (!strings.Contains(want, "-") && baseOf(got) == want))
}

// LanguageTag is the form a language takes in file names, as players read
// it: a region in capitals (pt-BR), a script with a capital first letter
// (zh-Hant), a number region as is (es-419), and Cantonese as yue.
func LanguageTag(code string) string {
	base, sub, ok := strings.Cut(code, "-")
	switch {
	case !ok:
		return code
	case code == "zh-yue":
		return "yue"
	case len(sub) == 4:
		return base + "-" + strings.ToUpper(sub[:1]) + sub[1:]
	}
	return base + "-" + strings.ToUpper(sub)
}

// LanguageName returns the English name for a two-letter code, or "" if unknown.
func LanguageName(code string) string {
	if l, ok := byCode[code]; ok {
		return l.name
	}
	return ""
}

// Language is one language the app knows, as shown in pickers.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Languages lists every language a wish can name.
func Languages() []Language {
	out := make([]Language, 0, len(languages))
	for _, l := range languages {
		out = append(out, Language{Code: l.two, Name: l.name})
	}
	return out
}

// ThreeLetter is a language's ISO 639-2 bibliographic code (eng, fre, ger),
// the form Jellyfin and Emby keep in their language settings; "" if unknown.
func ThreeLetter(code string) string {
	if l, ok := byCode[code]; ok {
		return l.threeB
	}
	return ""
}
