package metadata

import (
	"strings"
)

// tmdbLogo is one logo in TMDB's images.
type tmdbLogo struct {
	FilePath string  `json:"file_path"`
	Vote     float64 `json:"vote_average"`
	Language *string `json:"iso_639_1"`
	Country  *string `json:"iso_3166_1"`
}

// LangScore ranks a picture's text language for an app language: 3 its
// language and region (pt-BR for pt-br), 2 its language with no region,
// 1 its language for another region (a zh-CN logo for zh-tw still beats
// English), 0 another language. TMDB's nb is Norwegian too.
func LangScore(appLang string, lang, country *string) int {
	if lang == nil {
		return 0
	}
	code, region := tmdbParts(strings.ToLower(appLang))
	if l := strings.ToLower(*lang); l != code && !(code == "no" && l == "nb") {
		return 0
	}
	switch {
	case country == nil || *country == "":
		return 2
	case strings.EqualFold(*country, region):
		return 3
	}
	return 1
}

// TMDBCode is TMDB's language code for an app language (pt for pt-br,
// tl for fil).
func TMDBCode(appLang string) string {
	code, _ := tmdbParts(strings.ToLower(appLang))
	return code
}

// OrigLogo is the Details.Logos key of the logo in the title's original
// language, shown when the reader's language has none.
const OrigLogo = "orig"

// otherLogos picks, for each app language other than English (English
// stays in Details.Logo), the best PNG logo: its region first, then the
// language, highest vote within a rank. orig adds the original language's
// logo under OrigLogo.
func otherLogos(logos []tmdbLogo, images, orig string) map[string]string {
	best := func(lang string) string {
		pick, score, vote := "", 0, -1.0
		for _, l := range logos {
			if !strings.HasSuffix(strings.ToLower(l.FilePath), ".png") {
				continue
			}
			if s := LangScore(lang, l.Language, l.Country); s > 0 && (s > score || s == score && l.Vote > vote) {
				pick, score, vote = l.FilePath, s, l.Vote
			}
		}
		if pick == "" {
			return ""
		}
		return images + "/w500" + pick
	}
	out := map[string]string{}
	for _, lang := range textLanguages {
		if lang == "en" {
			continue
		}
		if p := best(lang); p != "" {
			out[lang] = p
		}
	}
	if orig = strings.ToLower(orig); orig != "" && orig != "en" {
		if p := best(orig); p != "" {
			out[OrigLogo] = p
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
