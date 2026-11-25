package sources

import (
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// sourceLanguagesKey keeps the Source's languages with a stored Release, so
// a held Release judged again still uses them.
const sourceLanguagesKey = "source_languages"

// addLanguages adds the languages in one Source value ("French", "fr", or a
// comma list as Newznab sends) that the reader knows.
func addLanguages(have []string, v string) []string {
	for _, part := range strings.Split(v, ",") {
		if code := mediafiles.LanguageCode(part); code != "" && !slices.Contains(have, code) {
			have = append(have, code)
		}
	}
	return have
}

// parseItem reads a result's name; languages the Source sends replace the
// ones read from the name, as Sonarr and Radarr do.
func parseItem(it Item, h releasename.Hints) releasename.Result {
	p := releasename.Parse(it.Title, h)
	if len(it.Languages) > 0 {
		p.Languages = slices.Clone(it.Languages)
	}
	return p
}

// storedSourceLanguages reads back the Source's languages kept with a Release.
func storedSourceLanguages(parsed map[string]any) []string {
	raw, _ := parsed[sourceLanguagesKey].([]any)
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
