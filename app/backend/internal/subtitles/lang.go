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
	{"eo", "epo", "epo", "Esperanto"}, {"es", "spa", "spa", "Spanish"}, {"et", "est", "est", "Estonian"},
	{"eu", "baq", "eus", "Basque"}, {"fa", "per", "fas", "Persian"}, {"fi", "fin", "fin", "Finnish"},
	{"fr", "fre", "fra", "French"}, {"ga", "gle", "gle", "Irish"}, {"gl", "glg", "glg", "Galician"},
	{"he", "heb", "heb", "Hebrew"}, {"hi", "hin", "hin", "Hindi"}, {"hr", "hrv", "hrv", "Croatian"},
	{"hu", "hun", "hun", "Hungarian"}, {"hy", "arm", "hye", "Armenian"}, {"id", "ind", "ind", "Indonesian"},
	{"is", "ice", "isl", "Icelandic"}, {"it", "ita", "ita", "Italian"}, {"ja", "jpn", "jpn", "Japanese"},
	{"ka", "geo", "kat", "Georgian"}, {"kk", "kaz", "kaz", "Kazakh"}, {"km", "khm", "khm", "Khmer"},
	{"ko", "kor", "kor", "Korean"}, {"ku", "kur", "kur", "Kurdish"}, {"lt", "lit", "lit", "Lithuanian"},
	{"lv", "lav", "lav", "Latvian"}, {"mk", "mac", "mkd", "Macedonian"}, {"ml", "mal", "mal", "Malayalam"},
	{"mn", "mon", "mon", "Mongolian"}, {"ms", "may", "msa", "Malay"}, {"my", "bur", "mya", "Burmese"},
	{"nb", "nob", "nob", "Norwegian Bokmål"}, {"ne", "nep", "nep", "Nepali"}, {"nl", "dut", "nld", "Dutch"},
	{"nn", "nno", "nno", "Norwegian Nynorsk"}, {"no", "nor", "nor", "Norwegian"}, {"pa", "pan", "pan", "Punjabi"},
	{"pl", "pol", "pol", "Polish"}, {"pt", "por", "por", "Portuguese"}, {"ro", "rum", "ron", "Romanian"},
	{"ru", "rus", "rus", "Russian"}, {"si", "sin", "sin", "Sinhala"}, {"sk", "slo", "slk", "Slovak"},
	{"sl", "slv", "slv", "Slovenian"}, {"sq", "alb", "sqi", "Albanian"}, {"sr", "srp", "srp", "Serbian"},
	{"sv", "swe", "swe", "Swedish"}, {"sw", "swa", "swa", "Swahili"}, {"ta", "tam", "tam", "Tamil"},
	{"te", "tel", "tel", "Telugu"}, {"th", "tha", "tha", "Thai"}, {"tl", "tgl", "tgl", "Tagalog"},
	{"tr", "tur", "tur", "Turkish"}, {"uk", "ukr", "ukr", "Ukrainian"}, {"ur", "urd", "urd", "Urdu"},
	{"uz", "uzb", "uzb", "Uzbek"}, {"vi", "vie", "vie", "Vietnamese"}, {"zh", "chi", "zho", "Chinese"},
}

var byCode = func() map[string]language {
	m := map[string]language{}
	for _, l := range languages {
		m[l.two], m[l.threeB], m[l.threeT] = l, l, l
		m[strings.ToLower(l.name)] = l
	}
	// Extra spellings that files and sites use.
	m["pob"], m["pb"] = m["pt"], m["pt"]
	m["iw"] = m["he"]
	m["in"] = m["id"]
	m["english"] = m["en"]
	return m
}()

// NormLanguage turns a language code or name into its two-letter code, so
// "eng", "en", "en-US" and "English" all give "en". It returns "" for unknown
// languages and for the special codes und, zxx, mul and mis.
func NormLanguage(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 40 {
		return ""
	}
	if i := strings.IndexAny(s, "-_"); i > 0 {
		s = s[:i] // drop a BCP 47 region or script
	}
	if l, ok := byCode[s]; ok {
		return l.two
	}
	return ""
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
