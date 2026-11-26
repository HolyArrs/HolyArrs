package discover

import (
	"context"
	"maps"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// readerLogo reads the logo for lang once per title and keeps it in
// d.Logos[lang] ("" when TMDB has none, so it is not asked again). It
// returns true when d changed and should be kept. d.Logos is replaced, not
// changed, so a cached copy stays as it was.
func readerLogo(ctx context.Context, src metadata.TitleLogoer, d *metadata.Details, kind, id, lang string) bool {
	if src == nil || metadata.TMDBCode(lang) == "en" {
		return false
	}
	if _, asked := d.Logos[lang]; asked {
		return false
	}
	logo, err := src.TitleLogo(ctx, kind, id, lang, d.OriginalLanguage)
	if err != nil {
		return false
	}
	logos := maps.Clone(d.Logos)
	if logos == nil {
		logos = map[string]string{}
	}
	logos[lang] = logo
	d.Logos = logos
	return true
}
