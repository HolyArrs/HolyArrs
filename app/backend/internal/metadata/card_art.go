package metadata

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// CardArt is the picture a wide title card shows, chosen once per title
// and language: a backdrop that already carries the title text, else a
// textless backdrop and the title logo, else the textless backdrop alone
// (the page then sets the name in its own type over the art).
type CardArt struct {
	Art      string `json:"card_art,omitempty"`
	Logo     string `json:"card_logo,omitempty"`
	HasTitle bool   `json:"art_has_title,omitempty"`
}

// CardImage is one image from a title's image list.
type CardImage struct {
	Path  string  `json:"file_path"`
	Lang  *string `json:"iso_639_1"`
	Vote  float64 `json:"vote_average"`
	Width int     `json:"width"`
	// Height is the native height in pixels (the sign-in art's quality gate).
	Height int `json:"height,omitempty"`
}

// CardImages is a title's backdrops and logos, as listed by the source.
type CardImages struct {
	Backdrops []CardImage `json:"backdrops"`
	Logos     []CardImage `json:"logos"`
	// Posters are listed for the sign-in art's phone (portrait) picture.
	Posters []CardImage `json:"posters,omitempty"`
}

// CardArter lists a title's images in one call.
type CardArter interface {
	CardImages(ctx context.Context, kind, tmdbID, lang string) (CardImages, error)
}

// CardArterFor returns the image list surface selected for ctx, when the
// chosen provider has one.
func CardArterFor(ctx context.Context, provider Provider) (CardArter, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	c, ok := provider.(CardArter)
	return c, ok
}

// CardImages answers GET /{movie|tv}/{id}/images for the language, English
// and the no-language images.
func (t *TMDB) CardImages(ctx context.Context, kind, tmdbID, lang string) (CardImages, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 || (kind != "movie" && kind != "show") {
		return CardImages{}, ErrNotFound
	}
	langs := "en,null"
	if lang = strings.ToLower(lang); len(lang) == 2 && lang != "en" {
		langs = lang + ",en,null"
	}
	b, err := t.get(ctx, "/"+tvPath(kind)+"/"+strconv.FormatInt(n, 10)+"/images", url.Values{"include_image_language": {langs}})
	if err != nil {
		return CardImages{}, err
	}
	var out CardImages
	if err := json.Unmarshal(b, &out); err != nil {
		return CardImages{}, err
	}
	for i := range out.Backdrops {
		out.Backdrops[i].Path = t.images() + "/w780" + out.Backdrops[i].Path
	}
	for i := range out.Logos {
		out.Logos[i].Path = t.images() + "/w500" + out.Logos[i].Path
	}
	for i := range out.Posters {
		out.Posters[i].Path = t.images() + "/w500" + out.Posters[i].Path
	}
	return out, nil
}

// PickCardArt chooses the card picture: the best textless backdrop (else the
// fallback, else a titled backdrop with no logo over it, so the name never shows
// twice). The logo (lang, else English, PNG only) floats over it. The source lists
// images best first; the highest vote wins within a group.
func PickCardArt(im CardImages, lang, fallback string) CardArt {
	lang = strings.ToLower(lang)
	if lang == "" {
		lang = "en"
	}
	best := func(list []CardImage, want string, png bool) string {
		pick, vote := "", -1.0
		for _, x := range list {
			if png && !strings.HasSuffix(strings.ToLower(x.Path), ".png") {
				continue
			}
			l := ""
			if x.Lang != nil {
				l = strings.ToLower(*x.Lang)
			}
			if l == want && x.Vote > vote {
				pick, vote = x.Path, x.Vote
			}
		}
		return pick
	}
	logo := ""
	for _, l := range uniq(lang, "en") {
		if logo = best(im.Logos, l, true); logo != "" {
			break
		}
	}
	// Netflix-style: always the clean (textless) backdrop, with the logo
	// floating over its bottom. A titled backdrop plus a logo would show the
	// name twice, so a titled backdrop is used only when nothing else exists.
	if art := best(im.Backdrops, "", false); art != "" {
		return CardArt{Art: art, Logo: logo}
	}
	if fallback != "" {
		return CardArt{Art: fallback, Logo: logo}
	}
	for _, l := range uniq(lang, "en") {
		if p := best(im.Backdrops, l, false); p != "" {
			return CardArt{Art: p, HasTitle: true}
		}
	}
	return CardArt{Logo: logo}
}

func uniq(a, b string) []string {
	if a == b {
		return []string{a}
	}
	return []string{a, b}
}
