package metadata

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// isoCode is a plain ISO 639 code, as TMDB lists original languages.
var isoCode = regexp.MustCompile(`^[a-z]{2,3}$`)

// logoAsk is the short, ordered language list a title logo read asks for:
// the reader's language, the title's original language, English, textless.
// TMDB only honours the first few codes of a long list.
func logoAsk(appLang, orig string) []string {
	out := []string{}
	add := func(c string) {
		if c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	add(TMDBCode(appLang))
	if o := strings.ToLower(orig); isoCode.MatchString(o) {
		add(o)
	}
	add("en")
	add("null")
	return out
}

// pickTitleLogo is the best PNG logo in the reader's language (its region
// first, highest vote), else in the original language; "" when neither
// has one, so the English or textless Details.Logo stays.
func pickTitleLogo(logos []tmdbLogo, lang, orig, images string) string {
	for _, want := range []string{lang, strings.ToLower(orig)} {
		if want == "" || TMDBCode(want) == "en" {
			continue
		}
		pick, score, vote := "", 0, -1.0
		for _, l := range logos {
			if !strings.HasSuffix(strings.ToLower(l.FilePath), ".png") {
				continue
			}
			if s := LangScore(want, l.Language, l.Country); s > 0 && (s > score || s == score && l.Vote > vote) {
				pick, score, vote = l.FilePath, s, l.Vote
			}
		}
		if pick != "" {
			return images + "/w500" + pick
		}
	}
	return ""
}

// TitleLogoer reads one title's logo for a reader's language.
type TitleLogoer interface {
	TitleLogo(ctx context.Context, kind, tmdbID, lang, orig string) (string, error)
}

// TitleLogoerFor returns the title logo surface selected for ctx.
func TitleLogoerFor(ctx context.Context, provider Provider) (TitleLogoer, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	l, ok := provider.(TitleLogoer)
	return l, ok
}

// TitleLogo answers GET /{movie|tv}/{id}/images with the short list from
// logoAsk and picks the reader's (else the original language's) logo.
func (t *TMDB) TitleLogo(ctx context.Context, kind, tmdbID, lang, orig string) (string, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 || (kind != "movie" && kind != "show") {
		return "", ErrNotFound
	}
	b, err := t.get(ctx, "/"+tvPath(kind)+"/"+strconv.FormatInt(n, 10)+"/images", url.Values{"include_image_language": {strings.Join(logoAsk(lang, orig), ",")}})
	if err != nil {
		return "", err
	}
	var out struct {
		Logos []tmdbLogo `json:"logos"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	return pickTitleLogo(out.Logos, strings.ToLower(lang), orig, t.images()), nil
}
