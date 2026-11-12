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
	{"pl", "pol", "pol", "Polish"}, {"pt", "por", "por", "Portuguese"}, {"pt-br", "pob", "pob", "Portuguese (Brazil)"},
	{"ro", "rum", "ron", "Romanian"},
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
	for _, s := range []string{"pb", "ptbr", "pt_br", "brazilian", "brazilian portuguese", "portuguese (brazilian)"} {
		m[s] = m["pt-br"]
	}
	m["iw"] = m["he"]
	m["in"] = m["id"]
	m["english"] = m["en"]
	return m
}()

// NormLanguage turns a language code or name into its code, so "eng", "en",
// "en-US" and "English" all give "en". Brazilian Portuguese keeps its own code
// pt-br; every other region or script is dropped. It returns "" for unknown
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
	for _, sub := range strings.FieldsFunc(s[i+1:], func(r rune) bool { return r == '-' || r == '_' }) {
		if l, ok := byCode[base+"-"+sub]; ok && base != "" {
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

// LanguageTag is the form a language takes in file names: the code with its
// region in capitals, such as pt-BR.
func LanguageTag(code string) string {
	if base, region, ok := strings.Cut(code, "-"); ok {
		return base + "-" + strings.ToUpper(region)
	}
	return code
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
