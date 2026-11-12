package metadata

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// A title page and Explore show a title's name, synopsis and tagline in the
// language each person uses the app in. The texts come with the title's
// details (TMDB translations, one answer for every language) and are kept
// per title; an empty text shows the English one. The Library record stays
// English: names, matching and file names never use these.

// TitleText is a title's name, synopsis and tagline in one language; an
// empty field has no text there, and the English one is shown.
type TitleText struct {
	Name     string `json:"name,omitempty"`
	Overview string `json:"overview,omitempty"`
	Tagline  string `json:"tagline,omitempty"`
}

// Apply puts the text's name and synopsis on it, keeping the English ones
// where the text has none.
func (x TitleText) Apply(it *DiscoverItem) {
	if x.Name != "" {
		it.Name = x.Name
	}
	if x.Overview != "" {
		it.Overview = x.Overview
	}
}

// textLanguages are the languages the app is shown in.
var textLanguages = []string{"ar", "bg", "bn", "ca", "cs", "da", "de", "el", "en", "es", "et", "fa", "fi", "fil", "fr", "he", "hi", "hr", "hu",
	"id", "is", "it", "ja", "ko", "lt", "lv", "ms", "nb", "nl", "pl", "pt-br", "pt-pt", "ro", "ru", "sk", "sr", "sv", "ta", "th", "tr", "uk", "vi",
	"zh-cn", "zh-tw"}

// tmdbRegion is the country whose TMDB text a language reads first, where
// it is not the language code itself (de-DE, fr-FR).
var tmdbRegion = map[string]string{"en": "US", "es": "ES", "pt-br": "BR", "pt-pt": "PT", "zh-cn": "CN", "zh-tw": "TW", "nb": "NO",
	"ar": "SA", "ja": "JP", "uk": "UA", "he": "IL", "el": "GR", "cs": "CZ", "da": "DK", "sv": "SE", "vi": "VN", "hi": "IN", "ko": "KR",
	"fil": "PH", "ca": "ES", "fa": "IR", "bn": "BD", "ta": "IN", "ms": "MY", "sr": "RS", "et": "EE"}

// tmdbNext is the country read second, before the language's others.
var tmdbNext = map[string]string{"zh-cn": "SG", "zh-tw": "HK"}

// appLanguage maps a language tag (es, es-MX, pt-PT, zh-Hant-HK, nn) to the
// app language it is shown in; "" when the app has none for it.
func appLanguage(tag string) string {
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")), "-")
	switch parts[0] {
	case "pt":
		if slices.Contains(parts[1:], "pt") {
			return "pt-pt"
		}
		return "pt-br"
	case "zh":
		for _, p := range parts[1:] {
			if p == "tw" || p == "hk" || p == "mo" || p == "hant" {
				return "zh-tw"
			}
		}
		return "zh-cn"
	case "no", "nn", "nb":
		return "nb"
	case "iw":
		return "he"
	case "in":
		return "id"
	case "tl":
		return "fil"
	case "sh":
		return "sr"
	}
	if slices.Contains(textLanguages, parts[0]) {
		return parts[0]
	}
	return ""
}

// TextLanguage is the app language the asking page is shown in: the app
// sends it as X-HolyArrs-Language; without it (or an odd one) the
// browser's Accept-Language counts, else English.
func TextLanguage(r *http.Request) string {
	if l := appLanguage(r.Header.Get("X-HolyArrs-Language")); l != "" {
		return l
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(part, ";")
		if l := appLanguage(tag); l != "" {
			return l
		}
	}
	return "en"
}

// tmdbParts are TMDB's language code and first country for an app language.
func tmdbParts(lang string) (code, country string) {
	code, _, _ = strings.Cut(lang, "-")
	switch code {
	case "nb":
		code = "no"
	case "fil":
		code = "tl"
	}
	if country = tmdbRegion[lang]; country == "" {
		country = strings.ToUpper(code)
	}
	return code, country
}

// TMDBLanguage is TMDB's language parameter for an app language (es-ES).
func TMDBLanguage(lang string) string {
	code, country := tmdbParts(lang)
	return code + "-" + country
}

// tmdbTranslation is one language of a title in TMDB's translations.
type tmdbTranslation struct {
	Lang    string `json:"iso_639_1"`
	Country string `json:"iso_3166_1"`
	Data    struct {
		Title    string `json:"title"` // movies
		Name     string `json:"name"`  // shows
		Overview string `json:"overview"`
		Tagline  string `json:"tagline"`
	} `json:"data"`
}

// titleTexts picks each app language's text from TMDB's translations: the
// language's first country, then its others, field by field. English is
// the record itself and is left out, as are languages with no text.
func titleTexts(list []tmdbTranslation) map[string]TitleText {
	out := map[string]TitleText{}
	for _, lang := range textLanguages {
		if lang == "en" {
			continue
		}
		code, first := tmdbParts(lang)
		rank := func(tr tmdbTranslation) int {
			switch c := strings.ToUpper(tr.Country); {
			case c == first:
				return 0
			case c == tmdbNext[lang]:
				return 1
			}
			return 2
		}
		var same []tmdbTranslation
		for _, tr := range list {
			if l := strings.ToLower(tr.Lang); l == code || code == "no" && l == "nb" {
				same = append(same, tr)
			}
		}
		slices.SortStableFunc(same, func(a, b tmdbTranslation) int { return rank(a) - rank(b) })
		var x TitleText
		for _, tr := range same {
			if x.Name == "" {
				x.Name = strings.TrimSpace(cmp.Or(tr.Data.Title, tr.Data.Name))
			}
			if x.Overview == "" {
				x.Overview = strings.TrimSpace(tr.Data.Overview)
			}
			if x.Tagline == "" {
				x.Tagline = strings.TrimSpace(tr.Data.Tagline)
			}
		}
		if x != (TitleText{}) {
			out[lang] = x
		}
	}
	return out
}

func textKey(kind, tmdbID string) string { return "title_text|" + kind + "|" + tmdbID }

// keepTexts keeps a title's texts in memory and on Disk, as long as other
// kept details.
func (t *TMDB) keepTexts(ctx context.Context, kind, tmdbID string, texts map[string]TitleText) {
	t.rememberTexts(kind, tmdbID, texts)
	if t.Disk == nil {
		return
	}
	if b, err := json.Marshal(texts); err == nil {
		now := time.Now()
		_ = t.Disk.Put(context.WithoutCancel(ctx), textKey(kind, tmdbID), b, now, now.Add(keptOnDisk))
	}
}

func (t *TMDB) rememberTexts(kind, tmdbID string, texts map[string]TitleText) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.texts == nil || len(t.texts) >= startup.LowMemoryPick(maxCache, 200) {
		t.texts = map[string]map[string]TitleText{}
	}
	t.texts[kind+"|"+tmdbID] = texts
}

// KeptText is a title's text in an app language, from what its details
// left (memory, then Disk); it never asks outside. ok is false when no
// details were read for the title yet; an empty text means English.
func (t *TMDB) KeptText(ctx context.Context, kind, tmdbID, lang string) (TitleText, bool) {
	t.mu.Lock()
	texts, ok := t.texts[kind+"|"+tmdbID]
	t.mu.Unlock()
	if !ok && t.Disk != nil {
		b, found, err := t.Disk.Get(ctx, textKey(kind, tmdbID), time.Now())
		if err == nil && found && json.Unmarshal(b, &texts) == nil {
			ok = true
			t.rememberTexts(kind, tmdbID, texts)
		}
	}
	return texts[lang], ok
}

// SeasonText is one season's episode names and synopses in an app language,
// by episode number (only episodes with text). English asks nothing.
func (t *TMDB) SeasonText(ctx context.Context, tmdbID string, season int, lang string) (map[int]TitleText, error) {
	if lang == "" || lang == "en" {
		return nil, nil
	}
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 || season < 0 || season > 999 {
		return nil, ErrNotFound
	}
	b, err := t.get(ctx, "/tv/"+strconv.FormatInt(n, 10)+"/season/"+strconv.Itoa(season), url.Values{"language": {TMDBLanguage(lang)}})
	if err != nil {
		return nil, err
	}
	var res struct {
		Episodes []struct {
			Number   int    `json:"episode_number"`
			Name     string `json:"name"`
			Overview string `json:"overview"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := map[int]TitleText{}
	for _, e := range res.Episodes {
		if x := (TitleText{Name: strings.TrimSpace(e.Name), Overview: strings.TrimSpace(e.Overview)}); x != (TitleText{}) {
			out[e.Number] = x
		}
	}
	return out, nil
}

// Texter reads a title's kept text in an app language.
type Texter interface {
	KeptText(ctx context.Context, kind, tmdbID, lang string) (TitleText, bool)
}

// TexterFor returns the text surface selected for ctx, when the chosen
// provider has one.
func TexterFor(ctx context.Context, provider Provider) (Texter, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	x, ok := provider.(Texter)
	return x, ok
}

// SeasonTexter reads a season's episode texts in an app language.
type SeasonTexter interface {
	SeasonText(ctx context.Context, tmdbID string, season int, lang string) (map[int]TitleText, error)
}

// SeasonTexterFor returns the season text surface selected for ctx.
func SeasonTexterFor(ctx context.Context, provider Provider) (SeasonTexter, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	x, ok := provider.(SeasonTexter)
	return x, ok
}
